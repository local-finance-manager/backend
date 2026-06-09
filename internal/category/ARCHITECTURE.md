# Arquitetura — Módulo `category`

## Responsabilidade

Gerencia categorias e subcategorias de transações financeiras pessoais.
Fornece CRUD completo, seed imutável do sistema (24 categorias / 142 subcategorias) e
endpoint de consulta de subcategorias por tipo.

---

## Estrutura de arquivos

```
internal/category/
├── ARCHITECTURE.md          # este documento
├── category.go              # domínio: entidades, tipos, inputs, erros, validação
├── repository.go            # interfaces de repositório (definidas no consumidor — ISP)
├── usecases.go              # interfaces de caso de uso
├── category_usecases.go     # implementações: List, Get, Create, Update, Delete (Category)
├── subcategory_usecases.go  # implementações: List, ListByType, Get, Create, Update, Delete (Sub)
├── sqlite.go                # adapters SQLite: SQLiteCategoryRepository + SQLiteSubcategoryRepository
├── handler.go               # HTTP handler + HandlerDeps + conversão domain → response
├── routes.go                # registro de rotas Chi v5
├── category_test.go         # testes de domínio (validações)
├── usecases_test.go         # testes de casos de uso com fakes + compile-time checks
└── sqlite_test.go           # testes de integração SQLite (:memory:)
```

---

## Camadas e dependências

```
HTTP (handler.go / routes.go)
        │  depende de
Use Cases (usecases.go + *_usecases.go)
        │  depende de
Repository Interfaces (repository.go)
        │  implementado por
SQLite Adapters (sqlite.go)
        │  depende de
domain (category.go) + shared (shared/pagination.go)
```

A camada de domínio não importa nada interno ao projeto.
Use cases importam apenas `shared` e `repository.go` (interface, nunca concreto).

---

## Decisão: dois structs de repositório

`CategoryRepository` e `SubcategoryRepository` têm métodos com o mesmo nome (`Get`,
`Create`, `Delete`) mas assinaturas diferentes. Em Go, um único struct não pode
implementar ambas as interfaces simultaneamente. Por isso existem dois adaptadores
separados que compartilham o mesmo `*sql.DB`:

```
SQLiteCategoryRepository    → implementa CategoryRepository
SQLiteSubcategoryRepository → implementa SubcategoryRepository
```

Em `main.go`, ambos recebem o mesmo `*sql.DB` aberto por `database.Open`.

---

## Padrão: use case structs individuais

Cada caso de uso é uma struct privada que implementa uma interface pública:

```go
// Interface pública — definida onde é consumida (ISP)
type CreateCategoryUseCase interface {
    Execute(ctx context.Context, in CreateCategoryInput) (Category, error)
}

// Implementação privada
type createCategoryImpl struct{ repo CategoryRepository }

func NewCreateCategory(repo CategoryRepository) CreateCategoryUseCase {
    return &createCategoryImpl{repo: repo}
}
```

Compile-time satisfaction verificada em `usecases_test.go`.

---

## Injeção de dependência via HandlerDeps

```go
type HandlerDeps struct {
    ListCategories          ListCategoriesUseCase
    GetCategory             GetCategoryUseCase
    CreateCategory          CreateCategoryUseCase
    UpdateCategory          UpdateCategoryUseCase
    DeleteCategory          DeleteCategoryUseCase
    ListSubcategories       ListSubcategoriesUseCase
    ListSubcategoriesByType ListSubcategoriesByTypeUseCase
    GetSubcategory          GetSubcategoryUseCase
    CreateSubcategory       CreateSubcategoryUseCase
    UpdateSubcategory       UpdateSubcategoryUseCase
    DeleteSubcategory       DeleteSubcategoryUseCase
}
```

Cada campo é uma interface — substituível por fake nos testes.

---

## Paginação

Categorias usam defaults específicos do módulo:
- `Limit = 500` (raramente há mais que isso)
- `OrderBy = "name"`, `Order = "ASC"` (usabilidade)

Subcategorias listadas por tipo (`/sub-categories?type=`) **não são paginadas** — retornam
lista completa (`{"data":[...]}`) pois são usadas para popular selects na UI.

---

## Ordenação segura no SQLite

`ORDER BY` usa interpolação de string apenas com valores provenientes do allowlist
validado por `shared.ParsePagination`. O `Order` é sempre normalizado para `"ASC"` ou
`"DESC"` pelo parser. Nenhum input de usuário é interpolado diretamente.

---

## Seed e imutabilidade

O seed (migração `0003`) usa `INSERT OR IGNORE` tornando-o idempotente.
Todos os registros do seed têm `can_be_deleted = 0`. As regras de negócio aplicadas:

- **DeleteCategory**: verifica `CanBeDeleted` e `HasSubcategories`
- **DeleteSubcategory**: verifica `CanBeDeleted`
- **UpdateCategory/UpdateSubcategory**: permite atualizar `Name`, `Icon`, `Color` mesmo em registros do sistema

---

## Tipos de erro (domainerr)

| Situação                              | Erro               | HTTP |
|---------------------------------------|--------------------|------|
| categoria/subcategoria não encontrada | `NewNotFound`      | 404  |
| campo inválido ou ausente             | `NewBadRequest`    | 400  |
| registro não deletável (sistema)      | `NewConflict`      | 409  |
| categoria tem subcategorias           | `NewConflict`      | 409  |
| erro inesperado                       | `NewInternal`      | 500  |

---

## Rotas registradas

| Método | Caminho                               | Handler                    |
|--------|---------------------------------------|----------------------------|
| GET    | /api/categories                       | ListCategories             |
| GET    | /api/categories/sub-categories        | ListSubcategoriesByType    |
| GET    | /api/categories/{id}                  | GetCategory                |
| POST   | /api/categories                       | CreateCategory             |
| PUT    | /api/categories/{id}                  | UpdateCategory             |
| DELETE | /api/categories/{id}                  | DeleteCategory             |
| GET    | /api/categories/{id}/subcategories    | ListSubcategories          |
| GET    | /api/subcategories/{id}               | GetSubcategory             |
| POST   | /api/subcategories                    | CreateSubcategory          |
| PUT    | /api/subcategories/{id}               | UpdateSubcategory          |
| DELETE | /api/subcategories/{id}               | DeleteSubcategory          |

> Chi v5 usa radix tree; literais têm prioridade sobre parâmetros.
> `/sub-categories` é registrado ANTES de `/{id}` para documentar a intenção.
