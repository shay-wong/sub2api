//go:build integration

package service

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestRefundAuditUpsertSurvivesConcurrentPostgresConflict(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	db := newPaymentAuditPostgres(t, ctx, "sub2api_refund_test")
	_, err := db.ExecContext(ctx, paymentAuditLogTableSQL)
	require.NoError(t, err)
	applyPaymentAuditIdempotencyMigration(t, ctx, db)

	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	client.PaymentAuditLog.Use(func(next dbent.Mutator) dbent.Mutator {
		return dbent.MutateFunc(func(ctx context.Context, mutation dbent.Mutation) (dbent.Value, error) {
			if mutation.Op().Is(dbent.OpCreate) {
				ready <- struct{}{}
				<-release
			}
			return next.Mutate(ctx, mutation)
		})
	})

	service := &PaymentService{}
	errCh := make(chan error, 2)
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func(detail string) {
			defer workers.Done()
			tx, txErr := client.Tx(ctx)
			if txErr != nil {
				errCh <- txErr
				return
			}
			if txErr = service.upsertRefundAuditJSON(ctx, tx.Client(), 42, "REFUND_PENDING", []byte(detail)); txErr != nil {
				_ = tx.Rollback()
				errCh <- txErr
				return
			}
			errCh <- tx.Commit()
		}(fmt.Sprintf(`{"attempt":%d}`, i))
	}
	<-ready
	<-ready
	close(release)
	workers.Wait()
	close(errCh)
	for workerErr := range errCh {
		require.NoError(t, workerErr)
	}

	var count int
	var detail string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT COUNT(*), MAX(detail)
		FROM payment_audit_logs
		WHERE order_id = $1 AND action = $2
	`, "42", "REFUND_PENDING").Scan(&count, &detail))
	require.Equal(t, 1, count)
	require.Contains(t, []string{`{"attempt":0}`, `{"attempt":1}`}, detail)
}

func TestPaymentAuditIdempotencyScopesOnPostgres(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	db := newPaymentAuditPostgres(t, ctx, "sub2api_audit_scope_test")
	_, err := db.ExecContext(ctx, paymentAuditLogTableSQL)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		CREATE UNIQUE INDEX idx_payment_audit_logs_order_action_uniq
		ON payment_audit_logs(order_id, action)
	`)
	require.NoError(t, err)
	applyPaymentAuditIdempotencyMigration(t, ctx, db)
	_, err = db.ExecContext(ctx, `
		INSERT INTO payment_audit_logs (order_id, action, detail, operator)
		VALUES ('legacy-writer', 'AFFILIATE_REBATE_APPLIED', '{}', 'system')
		ON CONFLICT (order_id, action) DO NOTHING
	`)
	require.NoError(t, err, "migration must retain the arbiter used by rolling old instances")

	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	service := &PaymentService{}

	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	require.NoError(t, service.upsertRefundAuditJSON(ctx, tx.Client(), 10, "REFUND_PENDING", []byte(`{"attempt":1}`)))
	require.NoError(t, service.upsertRefundAuditJSON(ctx, tx.Client(), 10, "REFUND_PENDING", []byte(`{"attempt":2}`)))
	require.NoError(t, tx.Commit())

	claimResults := make(chan bool, 2)
	claimErrors := make(chan error, 2)
	var claims sync.WaitGroup
	for i := 0; i < 2; i++ {
		claims.Add(1)
		go func() {
			defer claims.Done()
			tx, txErr := client.Tx(ctx)
			if txErr != nil {
				claimErrors <- txErr
				return
			}
			claimed, claimErr := service.tryClaimAffiliateRebateAudit(ctx, tx.Client(), 20, 100)
			if claimErr != nil {
				_ = tx.Rollback()
				claimErrors <- claimErr
				return
			}
			if txErr = tx.Commit(); txErr != nil {
				claimErrors <- txErr
				return
			}
			claimResults <- claimed
		}()
	}
	claims.Wait()
	close(claimErrors)
	close(claimResults)
	for claimErr := range claimErrors {
		require.NoError(t, claimErr)
	}
	claimedCount := 0
	for claimed := range claimResults {
		if claimed {
			claimedCount++
		}
	}
	require.Equal(t, 1, claimedCount)

	_, err = client.PaymentAuditLog.Create().
		SetOrderID("30").
		SetAction("PAYMENT_INVALID_AMOUNT").
		SetDetail("event-0").
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Create().
		SetOrderID("30").
		SetAction("PAYMENT_INVALID_AMOUNT").
		SetDetail("event-1").
		SetOperator("system").
		Save(ctx)
	require.True(t, dbent.IsConstraintError(err), "expand phase keeps legacy uniqueness until old binaries are drained")

	var refundCount, rebateCount, ordinaryCount int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM payment_audit_logs WHERE order_id = '10' AND action = 'REFUND_PENDING'").Scan(&refundCount))
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM payment_audit_logs WHERE order_id = '20' AND action IN ('AFFILIATE_REBATE_APPLIED', 'AFFILIATE_REBATE_SKIPPED')").Scan(&rebateCount))
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM payment_audit_logs WHERE order_id = '30' AND action = 'PAYMENT_INVALID_AMOUNT'").Scan(&ordinaryCount))
	require.Equal(t, 1, refundCount)
	require.Equal(t, 1, rebateCount)
	require.Equal(t, 1, ordinaryCount)
}

func TestPaymentAuditExpandMigrationDeduplicatesAffiliateClaims(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	db := newPaymentAuditPostgres(t, ctx, "sub2api_audit_migration_test")
	_, err := db.ExecContext(ctx, paymentAuditLogTableSQL)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		CREATE UNIQUE INDEX idx_payment_audit_logs_order_action_uniq
		ON payment_audit_logs(order_id, action)
	`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO payment_audit_logs (order_id, action, detail, created_at) VALUES
			('refund', 'REFUND_PENDING', 'current', '2026-08-02T00:00:00Z'),
			('rebate', 'AFFILIATE_REBATE_SKIPPED', 'skipped', '2026-08-02T00:00:00Z'),
			('rebate', 'AFFILIATE_REBATE_APPLIED', 'applied', '2026-08-01T00:00:00Z'),
			('subscription', 'SUBSCRIPTION_ASSIGNED', 'new', '2026-08-02T00:00:00Z'),
			('ordinary', 'FULFILLMENT_FAILED', 'current', '2026-08-02T00:00:00Z')
	`)
	require.NoError(t, err)

	applyPaymentAuditIdempotencyMigration(t, ctx, db)
	applyPaymentAuditIdempotencyMigration(t, ctx, db)

	var refundDetail, rebateAction, subscriptionDetail string
	var ordinaryCount int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT detail FROM payment_audit_logs WHERE order_id = 'refund' AND action = 'REFUND_PENDING'").Scan(&refundDetail))
	require.NoError(t, db.QueryRowContext(ctx, "SELECT action FROM payment_audit_logs WHERE order_id = 'rebate'").Scan(&rebateAction))
	require.NoError(t, db.QueryRowContext(ctx, "SELECT detail FROM payment_audit_logs WHERE order_id = 'subscription' AND action = 'SUBSCRIPTION_ASSIGNED'").Scan(&subscriptionDetail))
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM payment_audit_logs WHERE order_id = 'ordinary'").Scan(&ordinaryCount))
	require.Equal(t, "current", refundDetail)
	require.Equal(t, "AFFILIATE_REBATE_APPLIED", rebateAction)
	require.Equal(t, "new", subscriptionDetail)
	require.Equal(t, 1, ordinaryCount)
}

func TestRefundFinalizationUsesLatestAuditAfterPostgresLock(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	db := newPaymentAuditPostgres(t, ctx, "sub2api_refund_finalize_test")
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	require.NoError(t, client.Schema.Create(ctx))

	t.Run("pending plus pending compensates once", func(t *testing.T) {
		order := createPostgresPendingRefundOrder(t, ctx, client, "pending-pending", "attempt-pending-pending")
		svc, adjustments, started, release := newBlockedRefundAdjustmentService(client)
		detail, err := svc.latestRefundPendingDetail(ctx, order.ID)
		require.NoError(t, err)

		first := make(chan error, 1)
		go func() {
			_, callErr := svc.rollbackPendingRefundDeduction(ctx, svc.refundFinalizePlan(order, detail), detail, nil)
			first <- callErr
		}()
		select {
		case <-started:
		case <-time.After(10 * time.Second):
			t.Fatal("first pending finalizer did not reach compensation")
		}
		second := make(chan error, 1)
		go func() {
			_, callErr := svc.rollbackPendingRefundDeduction(ctx, svc.refundFinalizePlan(order, detail), detail, nil)
			second <- callErr
		}()
		close(release)

		require.NoError(t, <-first)
		require.NoError(t, <-second)
		adjustCalls, deductCalls := adjustments.counts()
		require.Equal(t, 1, adjustCalls)
		require.Zero(t, deductCalls)
		reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
		require.NoError(t, err)
		require.Equal(t, OrderStatusRefundPending, reloaded.Status)
	})

	t.Run("pending plus success compensates then deducts", func(t *testing.T) {
		order := createPostgresPendingRefundOrder(t, ctx, client, "pending-success", "attempt-pending-success")
		svc, adjustments, started, release := newBlockedRefundAdjustmentService(client)
		detail, err := svc.latestRefundPendingDetail(ctx, order.ID)
		require.NoError(t, err)

		pending := make(chan error, 1)
		go func() {
			_, callErr := svc.rollbackPendingRefundDeduction(ctx, svc.refundFinalizePlan(order, detail), detail, nil)
			pending <- callErr
		}()
		select {
		case <-started:
		case <-time.After(10 * time.Second):
			t.Fatal("pending finalizer did not reach compensation")
		}
		type successOutcome struct {
			result *RefundResult
			err    error
		}
		success := make(chan successOutcome, 1)
		go func() {
			result, callErr := svc.finalizePendingRefundSuccess(ctx, svc.refundFinalizePlan(order, detail))
			success <- successOutcome{result: result, err: callErr}
		}()
		close(release)

		require.NoError(t, <-pending)
		outcome := <-success
		require.NoError(t, outcome.err)
		require.NotNil(t, outcome.result)
		require.True(t, outcome.result.Success)
		adjustCalls, deductCalls := adjustments.counts()
		require.Equal(t, 1, adjustCalls)
		require.Equal(t, 1, deductCalls)
		reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
		require.NoError(t, err)
		require.Equal(t, OrderStatusRefunded, reloaded.Status)
	})
}

func TestFullRefundDeductionCompensationMergesConcurrentRenewalOnPostgres(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	db := newPaymentAuditPostgres(t, ctx, "sub2api_refund_subscription_race_test")
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	require.NoError(t, client.Schema.Create(ctx))

	project, err := client.Project.Create().SetName("refund race").SetSlug("refund-race").Save(ctx)
	require.NoError(t, err)
	user, err := client.User.Create().SetEmail("refund-race@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetProjectID(project.ID).
		SetName("refund race subscription").
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)
	originalExpiry := time.Now().UTC().AddDate(0, 0, 5).Truncate(time.Microsecond)
	created, err := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStartsAt(time.Now().UTC().AddDate(0, 0, -1)).
		SetExpiresAt(originalExpiry).
		SetStatus(SubscriptionStatusActive).
		SetAssignedAt(time.Now().UTC()).
		SetNotes("").
		Save(ctx)
	require.NoError(t, err)

	deductionRepo := &postgresRefundSubscriptionRepo{client: client}
	deductionSvc := NewSubscriptionService(nil, deductionRepo, nil, client, nil)
	t.Cleanup(deductionSvc.Stop)
	plan := &RefundPlan{
		Order:           &dbent.PaymentOrder{UserID: user.ID},
		DeductionType:   payment.DeductionTypeSubscription,
		SubDaysToDeduct: 30,
		SubscriptionID:  created.ID,
	}
	_, err = (&PaymentService{subscriptionSvc: deductionSvc}).applyRefundSubscriptionDeduction(ctx, plan, true)
	require.NoError(t, err)
	require.True(t, plan.SubscriptionRevoked)
	require.Equal(t, originalExpiry, plan.SubscriptionExpiresAtBeforeDeduction)

	renewalLocked := make(chan struct{})
	releaseRenewal := make(chan struct{})
	renewalRepo := &postgresRefundSubscriptionRepo{client: client, onUpdate: func() {
		close(renewalLocked)
		<-releaseRenewal
	}}
	renewalSvc := NewSubscriptionService(&fixedRefundSubscriptionGroupRepo{group: &Group{ID: group.ID, SubscriptionType: SubscriptionTypeSubscription}}, renewalRepo, nil, client, nil)
	t.Cleanup(renewalSvc.Stop)
	type renewalOutcome struct {
		sub *UserSubscription
		err error
	}
	renewalResult := make(chan renewalOutcome, 1)
	go func() {
		sub, _, renewErr := renewalSvc.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{UserID: user.ID, GroupID: group.ID, ValidityDays: 10})
		renewalResult <- renewalOutcome{sub: sub, err: renewErr}
	}()
	<-renewalLocked

	compensationStarted := make(chan struct{})
	compensationRepo := &postgresRefundSubscriptionRepo{client: client, beforeForUpdate: func() { close(compensationStarted) }}
	compensationSvc := NewSubscriptionService(nil, compensationRepo, nil, client, nil)
	t.Cleanup(compensationSvc.Stop)
	compensationResult := make(chan error, 1)
	go func() {
		_, compensateErr := (&PaymentService{subscriptionSvc: compensationSvc}).rollbackRefund(ctx, plan, true)
		compensationResult <- compensateErr
	}()
	<-compensationStarted
	close(releaseRenewal)

	renewed := <-renewalResult
	require.NoError(t, renewed.err)
	require.NoError(t, <-compensationResult)
	finalSub, err := compensationRepo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, SubscriptionStatusActive, finalSub.Status)
	require.WithinDuration(t, renewed.sub.ExpiresAt.Add(plan.SubscriptionExpiresAtBeforeDeduction.Sub(plan.SubscriptionExpiresAtAfterDeduction)), finalSub.ExpiresAt, time.Microsecond)
	count, err := client.UserSubscription.Query().Where(usersubscription.UserIDEQ(user.ID), usersubscription.GroupIDEQ(group.ID)).Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

type postgresRefundSubscriptionRepo struct {
	UserSubscriptionRepository
	client          *dbent.Client
	beforeForUpdate func()
	onUpdate        func()
}

func (r *postgresRefundSubscriptionRepo) entClient(ctx context.Context) *dbent.Client {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return tx.Client()
	}
	return r.client
}

func (r *postgresRefundSubscriptionRepo) getByID(ctx context.Context, id int64, forUpdate bool) (*UserSubscription, error) {
	query := r.entClient(ctx).UserSubscription.Query().Where(usersubscription.IDEQ(id))
	if forUpdate {
		if r.beforeForUpdate != nil {
			r.beforeForUpdate()
		}
		query = query.ForUpdate()
	}
	row, err := query.Only(ctx)
	if err != nil {
		return nil, err
	}
	return refundSubscriptionFromEnt(row), nil
}

func (r *postgresRefundSubscriptionRepo) GetByID(ctx context.Context, id int64) (*UserSubscription, error) {
	return r.getByID(ctx, id, false)
}

func (r *postgresRefundSubscriptionRepo) GetByIDForUpdate(ctx context.Context, id int64) (*UserSubscription, error) {
	return r.getByID(ctx, id, true)
}

func (r *postgresRefundSubscriptionRepo) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*UserSubscription, error) {
	row, err := r.entClient(ctx).UserSubscription.Query().
		Where(usersubscription.UserIDEQ(userID), usersubscription.GroupIDEQ(groupID)).
		Only(ctx)
	if err != nil {
		return nil, err
	}
	return refundSubscriptionFromEnt(row), nil
}

func (r *postgresRefundSubscriptionRepo) Create(ctx context.Context, sub *UserSubscription) error {
	created, err := r.entClient(ctx).UserSubscription.Create().
		SetUserID(sub.UserID).
		SetGroupID(sub.GroupID).
		SetStartsAt(sub.StartsAt).
		SetExpiresAt(sub.ExpiresAt).
		SetStatus(sub.Status).
		SetAssignedAt(sub.AssignedAt).
		SetNotes(sub.Notes).
		Save(ctx)
	if err == nil {
		sub.ID = created.ID
	}
	return err
}

func (r *postgresRefundSubscriptionRepo) Update(ctx context.Context, sub *UserSubscription) error {
	if r.onUpdate != nil {
		r.onUpdate()
	}
	_, err := r.entClient(ctx).UserSubscription.UpdateOneID(sub.ID).
		SetStartsAt(sub.StartsAt).
		SetExpiresAt(sub.ExpiresAt).
		SetStatus(sub.Status).
		SetAssignedAt(sub.AssignedAt).
		SetNotes(sub.Notes).
		Save(ctx)
	return err
}

func (r *postgresRefundSubscriptionRepo) ExtendExpiry(ctx context.Context, subscriptionID int64, expiresAt time.Time) error {
	_, err := r.entClient(ctx).UserSubscription.UpdateOneID(subscriptionID).SetExpiresAt(expiresAt).Save(ctx)
	return err
}

func (r *postgresRefundSubscriptionRepo) UpdateStatus(ctx context.Context, subscriptionID int64, status string) error {
	_, err := r.entClient(ctx).UserSubscription.UpdateOneID(subscriptionID).SetStatus(status).Save(ctx)
	return err
}

func refundSubscriptionFromEnt(row *dbent.UserSubscription) *UserSubscription {
	return &UserSubscription{
		ID: row.ID, UserID: row.UserID, GroupID: row.GroupID,
		StartsAt: row.StartsAt, ExpiresAt: row.ExpiresAt, Status: row.Status,
		AssignedAt: row.AssignedAt, Notes: psStringValue(row.Notes),
	}
}

type fixedRefundSubscriptionGroupRepo struct {
	GroupRepository
	group *Group
}

func (r *fixedRefundSubscriptionGroupRepo) GetByID(context.Context, int64) (*Group, error) {
	return r.group, nil
}

type refundAdjustmentCounter struct {
	UserRepository
	mu          sync.Mutex
	adjustCalls int
	deductCalls int
	onAdjust    func()
}

func (r *refundAdjustmentCounter) AdjustBalance(ctx context.Context, _ int64, delta float64) (BalanceChange, error) {
	if dbent.TxFromContext(ctx) == nil {
		return BalanceChange{}, fmt.Errorf("refund compensation must use the order transaction")
	}
	r.mu.Lock()
	r.adjustCalls++
	onAdjust := r.onAdjust
	r.mu.Unlock()
	if onAdjust != nil {
		onAdjust()
	}
	return BalanceChange{New: delta}, nil
}

func (r *refundAdjustmentCounter) DeductAvailableBalance(ctx context.Context, _ int64, amount float64) (float64, error) {
	if dbent.TxFromContext(ctx) == nil {
		return 0, fmt.Errorf("refund deduction must use the order transaction")
	}
	r.mu.Lock()
	r.deductCalls++
	r.mu.Unlock()
	return amount, nil
}

func (r *refundAdjustmentCounter) counts() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.adjustCalls, r.deductCalls
}

func newBlockedRefundAdjustmentService(client *dbent.Client) (*PaymentService, *refundAdjustmentCounter, <-chan struct{}, chan<- struct{}) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	adjustments := &refundAdjustmentCounter{onAdjust: func() {
		once.Do(func() { close(started) })
		<-release
	}}
	return &PaymentService{entClient: client, userRepo: adjustments}, adjustments, started, release
}

func createPostgresPendingRefundOrder(t *testing.T, ctx context.Context, client *dbent.Client, suffix, attemptID string) *dbent.PaymentOrder {
	t.Helper()
	user, err := client.User.Create().
		SetEmail(suffix + "@example.com").
		SetPasswordHash("hash").
		SetUsername(suffix).
		SetBalance(100).
		Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(100).
		SetPayAmount(100).
		SetRechargeCode("REFUND-" + suffix).
		SetOutTradeNo("sub2_" + suffix).
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("pi_" + suffix).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusRefundPending).
		SetRefundAmount(100).
		SetRefundReason("pending refund").
		SetForceRefund(true).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Create().
		SetOrderID(fmt.Sprintf("%d", order.ID)).
		SetAction("REFUND_PENDING").
		SetOperator("admin").
		SetDetail(fmt.Sprintf(`{"attemptID":%q,"phase":"provider_confirmed","previousStatus":"COMPLETED","refundID":"rf_test","refundAmount":100,"reason":"pending refund","force":true,"deductBalance":true,"deductionApplied":true,"deductionType":"balance","balanceToDeduct":100,"deductionRollbackOK":false}`, attemptID)).
		Save(ctx)
	require.NoError(t, err)
	return order
}

func newPaymentAuditPostgres(t *testing.T, ctx context.Context, database string) *sql.DB {
	t.Helper()
	container, err := tcpostgres.Run(
		ctx,
		"postgres:18.1-alpine3.23",
		tcpostgres.WithDatabase(database),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(ctx)) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable", "TimeZone=UTC")
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func applyPaymentAuditIdempotencyMigration(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	migrationSQL, err := dbmigrations.FS.ReadFile("194_payment_audit_action_idempotency_scopes.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
}

const paymentAuditLogTableSQL = `
CREATE TABLE payment_audit_logs (
	id BIGSERIAL PRIMARY KEY,
	order_id VARCHAR(64) NOT NULL,
	action VARCHAR(50) NOT NULL,
	detail TEXT NOT NULL DEFAULT '',
	operator VARCHAR(100) NOT NULL DEFAULT 'system',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`
