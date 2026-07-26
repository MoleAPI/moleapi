package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedDeletableSubscriptionPlan(t *testing.T, id int) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{
		Id:            id,
		Title:         "Delete Me",
		PriceAmount:   10,
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		TotalAmount:   1000,
	}
	require.NoError(t, DB.Create(plan).Error)
	return plan
}

func TestAdminDeleteSubscriptionPlanDeletesUnusedPlan(t *testing.T) {
	truncateTables(t)

	plan := seedDeletableSubscriptionPlan(t, 9701)
	deleted, err := AdminDeleteSubscriptionPlan(plan.Id)

	require.NoError(t, err)
	require.NotNil(t, deleted)
	assert.Equal(t, plan.Id, deleted.Id)

	var count int64
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestAdminDeleteSubscriptionPlanRejectsReferencedPlan(t *testing.T) {
	tests := []struct {
		name string
		seed func(t *testing.T, plan *SubscriptionPlan)
	}{
		{
			name: "user subscription",
			seed: func(t *testing.T, plan *SubscriptionPlan) {
				t.Helper()
				require.NoError(t, DB.Create(&UserSubscription{
					UserId:      501,
					PlanId:      plan.Id,
					AmountTotal: 1000,
					StartTime:   time.Now().Unix(),
					EndTime:     time.Now().Add(time.Hour).Unix(),
					Status:      "active",
				}).Error)
			},
		},
		{
			name: "payment order",
			seed: func(t *testing.T, plan *SubscriptionPlan) {
				t.Helper()
				require.NoError(t, DB.Create(&SubscriptionOrder{
					UserId:          502,
					PlanId:          plan.Id,
					Money:           10,
					TradeNo:         "delete-plan-order",
					PaymentMethod:   PaymentProviderStripe,
					PaymentProvider: PaymentProviderStripe,
					Status:          common.TopUpStatusPending,
					CreateTime:      time.Now().Unix(),
				}).Error)
			},
		},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateTables(t)
			plan := seedDeletableSubscriptionPlan(t, 9710+index)
			tt.seed(t, plan)

			deleted, err := AdminDeleteSubscriptionPlan(plan.Id)

			require.Error(t, err)
			assert.Nil(t, deleted)
			assert.Contains(t, err.Error(), "无法删除")

			var count int64
			require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Count(&count).Error)
			assert.EqualValues(t, 1, count)
		})
	}
}
