package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/local-finance-manager/backend/internal/backup"
	"github.com/local-finance-manager/backend/internal/category"
	"github.com/local-finance-manager/backend/internal/config"
	"github.com/local-finance-manager/backend/internal/creditcard"
	"github.com/local-finance-manager/backend/internal/database"
	"github.com/local-finance-manager/backend/internal/installment"
	"github.com/local-finance-manager/backend/internal/middleware"
	"github.com/local-finance-manager/backend/internal/transaction"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Subcomando único de autorização OAuth do Drive (D17): roda no host, com browser,
	// e grava o token.json. Depois disso o backup autentica sozinho.
	if len(os.Args) > 1 && os.Args[1] == "-authorize" {
		if err := runAuthorize(log); err != nil {
			log.Error("authorize", "error", err)
			os.Exit(1)
		}
		return
	}

	if err := run(log); err != nil {
		log.Error("fatal", "error", err)
		os.Exit(1)
	}
}

// stdoutPrinter satisfaz a interface esperada por backup.Authorize.
type stdoutPrinter struct{}

func (stdoutPrinter) Println(a ...any) { fmt.Println(a...) }

func runAuthorize(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.Backup.ClientID == "" || cfg.Backup.ClientSecret == "" {
		return errors.New("DRIVE_CLIENT_ID e DRIVE_CLIENT_SECRET são obrigatórios no .env para autorizar")
	}
	conf := backup.OAuthConfig(cfg.Backup.ClientID, cfg.Backup.ClientSecret, backup.LoopbackRedirectURL())
	tok, err := backup.Authorize(context.Background(), conf, stdoutPrinter{})
	if err != nil {
		return err
	}
	if err := backup.SaveToken(cfg.Backup.TokenPath, tok); err != nil {
		return err
	}
	log.Info("autorização concluída", "token_path", cfg.Backup.TokenPath)
	return nil
}

// restarter reinicia o processo após uma restauração (D7). Com restart:unless-stopped,
// o Docker re-sobe o container, que abre o banco já restaurado (ApplyPendingRestore).
type restarter struct{ log *slog.Logger }

func (r restarter) Restart() {
	r.log.Info("backup: reiniciando para aplicar restauração")
	go func() {
		time.Sleep(800 * time.Millisecond)
		os.Exit(0)
	}()
}

func runAutosave(ctx context.Context, svc *backup.Service, interval time.Duration, log *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			actx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if _, err := svc.Run(actx); err != nil { // dois tiers (Drive + local), só-se-mudou
				log.Warn("autosave", "error", err)
			}
			cancel()
		}
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Backup: prepara o cliente de Drive e resolve a versão do banco ANTES de abri-lo.
	backupStore := backup.NewFileStateStore(filepath.Join(cfg.Backup.DataDir, "backup-state.json"))
	driveClient, backupEnabled := buildDriveClient(cfg.Backup, log)

	// D7: aplica restauração pendente (deixada por um restore em runtime).
	if applied, aerr := backup.ApplyPendingRestore(cfg.Database.Path); aerr != nil {
		log.Warn("backup: apply pending restore", "error", aerr)
	} else if applied {
		log.Info("backup: restauração pendente aplicada")
	}

	// D16: "mais recente vence" — baixa o remoto se for mais novo, antes do Open.
	// Best-effort com timeout: offline/falha → segue com o banco local (RNF-BKP-13).
	if backupEnabled {
		syncCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		backup.SyncOnBoot(syncCtx, driveClient, backupStore, cfg.Backup, cfg.Database.Path, log)
		cancel()
	}

	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := database.RunMigrations(db); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	catRepo := category.NewSQLiteCategoryRepository(db)
	subRepo := category.NewSQLiteSubcategoryRepository(db)

	getSub := category.NewGetSubcategory(subRepo)
	getCat := category.NewGetCategory(catRepo)
	catFacade := category.NewSubcategoryFacade(getSub, getCat)

	categoryHandler := category.NewHandler(category.HandlerDeps{
		ListCategories:          category.NewListCategories(catRepo),
		GetCategory:             getCat,
		CreateCategory:          category.NewCreateCategory(catRepo),
		UpdateCategory:          category.NewUpdateCategory(catRepo),
		DeleteCategory:          category.NewDeleteCategory(catRepo),
		ListSubcategories:       category.NewListSubcategories(catRepo, subRepo),
		ListSubcategoriesByType: category.NewListSubcategoriesByType(subRepo),
		GetSubcategory:          getSub,
		CreateSubcategory:       category.NewCreateSubcategory(catRepo, subRepo),
		UpdateSubcategory:       category.NewUpdateSubcategory(subRepo),
		DeleteSubcategory:       category.NewDeleteSubcategory(subRepo),
	})

	// Cartão de crédito: repos + facades cross-module.
	ccRepo := creditcard.NewSQLiteCreditCardRepository(db)
	ccPayRepo := creditcard.NewSQLiteInvoicePaymentRepository(db)
	cardChecker := creditcard.NewCreditCardChecker(ccRepo) // transaction valida vínculo via este facade
	cardReader := transaction.NewCardReader(db)            // creditcard lê lançamentos via este facade

	transactionRepo := transaction.NewSQLiteRepository(db)
	transactionHandler := transaction.NewHandler(transaction.HandlerDeps{
		GetTransaction:     transaction.NewGetTransaction(transactionRepo),
		ListTransactions:   transaction.NewListTransactions(transactionRepo),
		CreateTransaction:  transaction.NewCreateTransaction(transactionRepo, catFacade, cardChecker),
		UpdateTransaction:  transaction.NewUpdateTransaction(transactionRepo, catFacade, cardChecker),
		ConfirmTransaction: transaction.NewConfirmTransaction(transactionRepo),
		DeleteTransaction:  transaction.NewDeleteTransaction(transactionRepo),
	})

	creditCardHandler := creditcard.NewHandler(creditcard.HandlerDeps{
		Create:       creditcard.NewCreateCreditCard(ccRepo),
		Get:          creditcard.NewGetCreditCard(ccRepo, ccPayRepo, cardReader),
		List:         creditcard.NewListCreditCards(ccRepo, ccPayRepo, cardReader),
		Update:       creditcard.NewUpdateCreditCard(ccRepo),
		Delete:       creditcard.NewDeleteCreditCard(ccRepo, cardReader),
		Archive:      creditcard.NewArchiveCreditCard(ccRepo),
		ListInvoices: creditcard.NewListInvoices(ccRepo, ccPayRepo, cardReader),
		GetInvoice:   creditcard.NewGetInvoice(ccRepo, ccPayRepo, cardReader),
		PayInvoice:   creditcard.NewPayInvoice(ccRepo, ccPayRepo, cardReader, catFacade),
		UndoPayment:  creditcard.NewUndoInvoicePayment(ccRepo, ccPayRepo, cardReader),
		MonthSummary: creditcard.NewMonthlyCardSummary(ccRepo, cardReader),
	})

	// Parcelamento: módulo installment reaproveita catFacade (despesa) e cardChecker
	// (cartão ativo); InvoiceReferenceFacade resolve a fatura de cada parcela.
	installmentHandler := installment.NewHandler(installment.NewService(installment.Deps{
		Repo:  installment.NewSQLiteRepository(db),
		Subs:  catFacade,
		Cards: cardChecker,
		Refs:  creditcard.NewInvoiceReferenceFacade(ccRepo),
	}))

	r := chi.NewRouter()

	// middleware.Error deve ser o primeiro: captura panics e formata qualquer
	// erro no padrão govalidator antes de qualquer outro handler rodar.
	r.Use(middleware.Error)
	r.Use(chiMiddleware.RequestID)
	r.Use(middleware.Logger(log))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"http://localhost:19741"},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Content-Type", "X-Request-ID"},
	}))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	backupSvc := backup.NewService(backup.Deps{
		Enabled:   backupEnabled,
		Cfg:       cfg.Backup,
		Drive:     driveClient,
		Snap:      backup.NewSQLiteSnapshotter(db),
		Store:     backupStore,
		Restarter: restarter{log: log},
		DBPath:    cfg.Database.Path,
		Log:       log,
	})

	r.Route("/api/categories", category.Routes(categoryHandler))
	r.Route("/api/subcategories", category.SubcategoryRoutes(categoryHandler))
	r.Route("/api/transactions", transaction.Routes(transactionHandler))
	r.Route("/api/credit-cards", creditcard.Routes(creditCardHandler))
	r.Route("/api/installments", installment.Routes(installmentHandler))
	r.Route("/api/backup", backup.Routes(backup.NewHandler(backupSvc)))

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		domainerr.WriteError(w, domainerr.NewNotFound("route not found"))
	})

	srv := &http.Server{
		Addr:         cfg.Addr(),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Autosave periódico (RF-BKP-17): roda se o intervalo > 0 e houver ALGUM tier ativo —
	// Drive habilitado OU o tier de snapshot local (que independe do Drive).
	if cfg.Backup.AutosaveInterval > 0 && (backupEnabled || cfg.Backup.LocalSnapshotEnabled) {
		go runAutosave(ctx, backupSvc, time.Duration(cfg.Backup.AutosaveInterval)*time.Minute, log)
	}

	go func() {
		log.Info("server starting", "addr", cfg.Addr())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("server shutdown", "error", err)
	}

	// D15/RF-BKP-06: backup best-effort no encerramento (não bloqueia além do timeout).
	bctx, bcancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer bcancel()
	backupSvc.BackupBestEffort(bctx)
	return nil
}

// buildDriveClient monta o cliente de Drive se a feature estiver configurada.
// Retorna (nil, false) e nunca falha o boot quando desabilitada/incompleta (RF-BKP-14).
func buildDriveClient(cfg config.BackupConfig, log *slog.Logger) (backup.DriveClient, bool) {
	if !cfg.Enabled || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, false
	}
	tok, err := backup.ResolveToken(cfg.TokenPath, cfg.RefreshToken)
	if err != nil || tok == nil {
		log.Warn("backup desabilitado: sem token (rode ./server -authorize ou preencha DRIVE_REFRESH_TOKEN)", "error", err)
		return nil, false
	}
	conf := backup.OAuthConfig(cfg.ClientID, cfg.ClientSecret, "")
	dc, err := backup.NewGoogleDriveClient(context.Background(), conf, tok)
	if err != nil {
		log.Warn("backup desabilitado: falha ao criar cliente do Drive", "error", err)
		return nil, false
	}
	return dc, true
}
