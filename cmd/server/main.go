package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/local-finance-manager/backend/internal/category"
	"github.com/local-finance-manager/backend/internal/config"
	"github.com/local-finance-manager/backend/internal/creditcard"
	"github.com/local-finance-manager/backend/internal/database"
	"github.com/local-finance-manager/backend/internal/middleware"
	"github.com/local-finance-manager/backend/internal/transaction"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(log); err != nil {
		log.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
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
		PayInvoice:   creditcard.NewPayInvoice(ccRepo, ccPayRepo, cardReader),
		UndoPayment:  creditcard.NewUndoInvoicePayment(ccRepo, ccPayRepo, cardReader),
		MonthSummary: creditcard.NewMonthlyCardSummary(ccRepo, cardReader),
	})

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

	r.Route("/api/categories", category.Routes(categoryHandler))
	r.Route("/api/subcategories", category.SubcategoryRoutes(categoryHandler))
	r.Route("/api/transactions", transaction.Routes(transactionHandler))
	r.Route("/api/credit-cards", creditcard.Routes(creditCardHandler))

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
	return srv.Shutdown(shutdownCtx)
}
