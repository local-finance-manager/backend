# Arquitetura — Módulo `backup`

## Responsabilidade

Protege os dados do usuário (app **100% local, single-user**) com **dois tiers** de backup,
ambos **só-se-mudou** (no-op por SHA-256):

1. **Drive (nuvem):** snapshot do `.sqlite` enviado ao Google Drive — uma cópia `latest`
   (sobrescrita) + N cópias datadas com retenção (default 5). Sincroniza no boot
   ("mais recente vence") e restaura sob demanda.
2. **Local (disco):** snapshots datados no **mesmo volume** do banco
   (`<DataDir>/snapshots/`), independentes de Drive/internet — a rede de segurança contra
   exclusão acidental/bug (RF-BKP-18). Retenção própria (default 5).

Gatilhos dos dois tiers: **Ctrl+S** (`POST /api/backup`), **autosave** periódico (default
15 min) e **shutdown** — todos via `Service.Run`. O snapshot é gerado por `VACUUM INTO`
(consistente com o banco aberto em WAL) e é **determinístico** (ver decisão abaixo).

---

## Estrutura de arquivos

```
internal/backup/
├── ARCHITECTURE.md   # este documento
├── backup.go         # domínio: tipos (BackupState, Status, Version, LocalSnapshot), erros, regras puras (ShouldUpload, VersionsToPrune, LocalSnapshotsToPrune, DatedFilename)
├── service.go        # aplicação: Backup, Restore, ListVersions, Status, Run (orquestrador), SyncOnBoot, BackupBestEffort
├── local.go          # aplicação: tier local — SnapshotLocal, ListLocalSnapshots, RestoreLocal, poda
├── ports.go          # interfaces consumidas: DriveClient, Snapshotter, StateStore, Restarter
├── drive.go          # adapter: DriveClient via SDK oficial do Google (mockável por httptest)
├── snapshot.go       # adapter: SQLiteSnapshotter (VACUUM INTO)
├── statestore.go     # adapter: FileStateStore (sidecar JSON backup-state.json)
├── auth.go           # OAuth: Authorize (loopback único), Load/Save/ResolveToken, OAuthConfig
├── fileutil.go       # helpers: hashFileBoth, writeAndHashSHA, copyFile, integrityCheck, isOffline
├── restore.go        # PendingRestorePath + ApplyPendingRestore (swap no boot)
├── handler.go        # HTTP handler + DTOs (snake_case)
└── routes.go         # registro das rotas chi
```

---

## Camadas e dependências

```
HTTP (handler.go / routes.go)
        │ depende de
Service (service.go + local.go)
        │ depende de (ports.go)
DriveClient · Snapshotter · StateStore · Restarter   ← injetados no main.go
        │ implementados por
drive.go (Google) · snapshot.go (VACUUM INTO) · statestore.go (JSON) · restarter (main.go)
```

O domínio (`backup.go`) é Go puro (tipos + regras testáveis sem I/O). O `Service` orquestra;
nunca conhece a SDK do Google (só o port `DriveClient`). O `main.go` é o único que monta os
adapters concretos. O `Snap`/`Store` são **sempre** injetados (mesmo com o Drive desligado) —
é o que permite o tier local funcionar offline.

---

## Decisão: snapshot determinístico (RNF-BKP2-01 — pré-condição do no-op)

O no-op compara o **SHA-256 do arquivo** do snapshot com o do último backup daquele tier
(`ShouldUpload`). Isso só se sustenta se `VACUUM INTO` for **determinístico**: comprovado
empiricamente (`TestSnapshotter_Determinism`) — o mesmo conteúdo lógico gera **bytes
idênticos** (inclusive após `insert+delete` que volta ao mesmo estado e após reabrir a
conexão). Uma mudança real gera um SHA diferente. **Se trocar o driver SQLite, re-rodar esse
teste.**

## Decisão: dois tiers + orquestrador `Run` (RF-BKP-16/18 / DA4)

`Run(ctx)` é o ponto de entrada dos três gatilhos. Executa o tier Drive (se habilitado) e o
tier local (se habilitado), cada um **só-se-mudou** e **best-effort entre si**: um Drive
offline **não** impede o snapshot local (fecha o buraco do incidente).

- **Erro/HTTP:** `ErrBackupDisabled` (409) só se **ambos** os tiers estão off. Drive offline
  com local OK → **200** (dado salvo localmente). Drive offline e sem tier local → **503**.
- **Resposta:** `Run` devolve um `BackupResult` (compatível com o Ctrl+S de hoje); quando só
  o local roda, sintetiza o resultado a partir do `LocalResult`.
- **Concorrência:** `Backup`/`Restore`/`SnapshotLocal`/`RestoreLocal` serializam em `s.mu`.
  `Run` **não** segura o lock — chama `Backup` e `SnapshotLocal` em sequência (Go não tem
  mutex reentrante), evitando lost-update no sidecar entre os campos Drive e local.

## Decisão: estado em sidecar JSON (não há banco)

`FileStateStore` grava `backup-state.json` no `DataDir`. `BackupState` tem o baseline do Drive
(`LastChecksumSHA256`, `Versions`…) **e** do local (`LocalLastChecksumSHA256`,
`LocalLastSnapshotAt`) — baselines independentes por tier. A **lista** de snapshots locais é
derivada do diretório (nomes datados, fonte da verdade), não do sidecar.

## Decisão: restauração via staging + restart (D7)

Restaurar (Drive ou local) **não** sobrescreve o banco em runtime: faz um **safety snapshot**
do estado atual, baixa/copia para `PendingRestorePath`, valida `integrity_check`, e marca
`RestartRequired`. O handler responde e **só então** chama `Restart()`; no próximo boot,
`ApplyPendingRestore` faz o swap atômico (limpando `-wal`/`-shm` órfãos).

---

## Tier local (RF-BKP-18) — `local.go`

- `<DataDir>/snapshots/financas-<data>.sqlite` (mesma partição → `os.Rename` atômico).
- `SnapshotLocal`: snapshot → hash → **no-op** se igual ao baseline local → senão promove
  (rename do tmp) → poda (`LocalSnapshotsToPrune`, nunca o mais novo) → atualiza o sidecar.
- `RestoreLocal`: **sanitiza o nome** (basename + padrão datado + existência — anti path
  traversal) antes de estagiar. Espelha o `Restore` do Drive.
- `ListLocalSnapshots`: lê o diretório, `CreatedAt` parseado do nome (autoritativo), `Size`
  via `os.Stat`, paginado.

---

## Configuração (env — RNF-BKP2-04)

| Var | Default | Significado |
|-----|---------|-------------|
| `DRIVE_SYNC_ENABLED` | `false` | liga o tier Drive |
| `DRIVE_BACKUP_RETENTION` | `5` | versões datadas no Drive (0 = só `latest`) |
| `BACKUP_AUTOSAVE_INTERVAL` | `15` | minutos do autosave (0 = desligado) |
| `LOCAL_SNAPSHOT_ENABLED` | `true` | liga o tier de snapshot local |
| `LOCAL_SNAPSHOT_RETENTION` | `5` | snapshots locais a manter (0 = desliga) |

> O autosave roda se o intervalo > 0 **e** houver algum tier ativo (Drive **ou** local).

---

## Tipos de erro (domainerr)

| Situação | Erro | HTTP |
|----------|------|------|
| ambos os tiers desabilitados | `ErrBackupDisabled` | 409 |
| sem conexão com o Drive (e sem fallback local) | `ErrDriveOffline` (sentinel → 503 no handler) | 503 |
| restauração sem confirmação | `ErrRestoreNotConfirmed` | 400 |
| versão (Drive) inexistente | `ErrVersionNotFound` | 404 |
| snapshot local inexistente | `ErrSnapshotNotFound` | 404 |
| nome de snapshot inválido (path traversal/formato) | `ErrInvalidSnapshotName` | 400 |
| checksum divergente após upload | `ErrChecksumMismatch` | 500 |

> O `govalidator` não expõe 503 — o `ErrDriveOffline` é um sentinel detectado no `handler.writeErr`.

---

## Rotas registradas

| Método | Caminho | Handler |
|--------|---------|---------|
| POST | /api/backup | Backup (→ `Run`: Drive + local, só-se-mudou) |
| GET | /api/backup/status | Status (inclui o tier local — RF-BKP-19) |
| GET | /api/backup/versions | ListVersions (Drive) |
| POST | /api/backup/restore | Restore (Drive) |
| GET | /api/backup/local-snapshots | ListLocalSnapshots |
| POST | /api/backup/restore-local | RestoreLocal |

> Restaurações respondem e **só então** disparam o restart (D7).
