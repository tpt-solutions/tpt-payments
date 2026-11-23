package payment_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
)

// Pages walk every intent exactly once, newest first, and the last page reports has_more=false.
func TestIntegration_ListIntents_PagesNewestFirst(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	var created []string
	for i := 0; i < 5; i++ {
		intent, err := e.svc.Create(ctx, e.scope(), payment.CreateParams{Amount: 1000, Currency: "usd"})
		require.NoError(t, err)
		created = append(created, intent.ID)
	}

	var seen []string
	cursor := ""
	for {
		page, err := e.svc.ListIntents(ctx, payment.ListParams{Scope: e.scope(), Limit: 2, StartingAfter: cursor})
		require.NoError(t, err)
		for _, i := range page.Intents {
			seen = append(seen, i.ID)
		}
		if !page.HasMore {
			break
		}
		cursor = page.Intents[len(page.Intents)-1].ID
	}

	require.Len(t, seen, 5)
	// Newest first: the reverse of creation order.
	for i := range created {
		assert.Equal(t, created[len(created)-1-i], seen[i])
	}
}

// Another merchant's intents are never listed, and their IDs are not usable as cursors. Both
// merchants live in the same schema, so the scoping is what keeps them apart.
func TestIntegration_ListIntents_IsScopedToMerchant(t *testing.T) {
	mine := setup(t)
	ctx := context.Background()
	mineIntent, err := mine.svc.Create(ctx, mine.scope(), payment.CreateParams{Amount: 500, Currency: "usd"})
	require.NoError(t, err)

	var otherMerchantID string
	require.NoError(t, mine.pool.QueryRow(ctx,
		`INSERT INTO merchants (name, email) VALUES ('Other Merchant', 'other@test.invalid') RETURNING id::text`,
	).Scan(&otherMerchantID))
	otherScope := payment.Scope{MerchantID: otherMerchantID}
	otherIntent, err := mine.svc.Create(ctx, otherScope, payment.CreateParams{Amount: 700, Currency: "usd"})
	require.NoError(t, err)

	page, err := mine.svc.ListIntents(ctx, payment.ListParams{Scope: mine.scope()})
	require.NoError(t, err)
	require.Len(t, page.Intents, 1)
	assert.Equal(t, mineIntent.ID, page.Intents[0].ID)

	_, err = mine.svc.ListIntents(ctx, payment.ListParams{Scope: mine.scope(), StartingAfter: otherIntent.ID})
	require.Error(t, err)
	var apiErr *apierrors.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 400, apiErr.StatusCode)
}

// The status filter returns only intents in that status.
func TestIntegration_ListIntents_FiltersByStatus(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, err := e.svc.Create(ctx, e.scope(), payment.CreateParams{Amount: 1000, Currency: "usd"})
	require.NoError(t, err)

	page, err := e.svc.ListIntents(ctx, payment.ListParams{Scope: e.scope(), Status: payment.StatusSucceeded})
	require.NoError(t, err)
	assert.Empty(t, page.Intents)
	page, err = e.svc.ListIntents(ctx, payment.ListParams{Scope: e.scope(), Status: payment.StatusRequiresPaymentMethod})
	require.NoError(t, err)
	assert.Len(t, page.Intents, 1)
}
