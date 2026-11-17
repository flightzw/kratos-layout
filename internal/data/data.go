package data

import (
	"context"

	"github.com/go-kratos/kratos/v3/log"
	"github.com/google/wire"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/flightzw/kratos-layout/internal/conf"
	"github.com/flightzw/kratos-layout/internal/data/query"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, NewTodoRepo)

type transactionKey struct{}

// Data holds the long-lived storage clients shared by repos.
type Data struct {
	db    *gorm.DB
	query *query.Query
}

// NewData opens the database client and returns it with a cleanup function.
func NewData(c *conf.Data) (*Data, func(), error) {
	dc := c.GetDatabase()
	db, err := gorm.Open(mysql.Open(dc.GetSource()), &gorm.Config{})
	if err != nil {
		return nil, nil, err
	}
	if dc.GetDebug() {
		db = db.Debug()
	}
	cleanup := func() {
		log.Info("closing the data resources")
		sqlDB, err := db.DB()
		if err != nil {
			log.Error("failed getting the sql.DB", "err", err)
			return
		}
		if err := sqlDB.Close(); err != nil {
			log.Error("failed closing the database", "err", err)
		}
	}
	return &Data{db: db, query: query.Use(db)}, cleanup, nil
}

// Transaction runs h in a transaction shared through its callback context.
// Nested calls use GORM savepoints on the current transaction.
func (d *Data) Transaction(ctx context.Context, h func(context.Context) error) error {
	return d.DB(ctx).Transaction(func(tx *gorm.DB) error {
		return h(context.WithValue(ctx, transactionKey{}, tx))
	})
}

// DB returns the current transaction or shared database with the request context.
func (d *Data) DB(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(transactionKey{}).(*gorm.DB); ok {
		return tx.WithContext(ctx)
	}
	return d.db.WithContext(ctx)
}

// Query returns the generated query client for the current transaction.
func (d *Data) Query(ctx context.Context) *query.Query {
	if tx, ok := ctx.Value(transactionKey{}).(*gorm.DB); ok {
		return query.Use(tx.WithContext(ctx))
	}
	return d.query
}
