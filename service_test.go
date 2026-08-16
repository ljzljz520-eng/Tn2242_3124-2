package fruitcut

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

type businessFixture struct {
	service  *OrderService
	clock    *FixedClock
	product  Product
	addOn    AddOn
	customer Customer
	store    Store
}

func newBusinessFixture(t *testing.T) businessFixture {
	t.Helper()
	location := time.FixedZone("UTC+8", 8*60*60)
	clock := NewFixedClock(time.Date(2026, time.August, 16, 8, 0, 0, 0, location))
	service := NewOrderService(NewMemoryStore(), clock, NewSequenceIDGenerator(1))

	customer, err := service.CreateCustomer(Customer{ID: "customer-lin", Name: "Lin", Phone: "13800000000"})
	if err != nil {
		t.Fatalf("customer should be available for ordering: %v", err)
	}
	store, err := service.CreateStore(Store{ID: "store-riverside", Name: "Riverside Fruit Cut", Address: "18 Riverside Road"})
	if err != nil {
		t.Fatalf("pickup store should be available for ordering: %v", err)
	}
	product, err := service.CreateProduct(Product{
		ID:           "product-watermelon",
		Name:         "Watermelon Cup",
		PricePerGram: decimal.RequireFromString("0.08"),
		StockGrams:   5000,
	})
	if err != nil {
		t.Fatalf("fruit product should be available for ordering: %v", err)
	}
	addOn, err := service.CreateAddOn(AddOn{
		ID:         "addon-chia",
		Name:       "Chia Seeds",
		Price:      decimal.RequireFromString("2.50"),
		StockUnits: 100,
	})
	if err != nil {
		t.Fatalf("add-on should be available for ordering: %v", err)
	}
	return businessFixture{
		service:  service,
		clock:    clock,
		product:  product,
		addOn:    addOn,
		customer: customer,
		store:    store,
	}
}

func (f businessFixture) pickupSlot() PickupSlot {
	start := time.Date(2026, time.August, 16, 11, 0, 0, 0, f.clock.Now().Location())
	return PickupSlot{Label: "11:00-11:30", Start: start, End: start.Add(30 * time.Minute)}
}

func (f businessFixture) createOrder(t *testing.T, id string, weight, addOnQuantity int, yogurt YogurtOption) Order {
	t.Helper()
	order, err := f.service.CreateOrder(DraftOrder{
		ID:         id,
		CustomerID: f.customer.ID,
		StoreID:    f.store.ID,
		Items: []OrderItem{{
			ProductID:   f.product.ID,
			WeightGrams: weight,
			Yogurt:      yogurt,
			AddOns:      []OrderAddOn{{AddOnID: f.addOn.ID, Quantity: addOnQuantity}},
		}},
		Pickup:        f.pickupSlot(),
		PaymentMethod: PaymentMethodMobilePay,
	})
	if err != nil {
		t.Fatalf("pickup order should be created: %v", err)
	}
	return order
}

func (f businessFixture) completeOrder(t *testing.T, id string) Order {
	t.Helper()
	f.clock.Set(time.Date(2026, time.August, 16, 8, 5, 0, 0, f.clock.Now().Location()))
	if _, err := f.service.PayOrder(id, "payment-001"); err != nil {
		t.Fatalf("pickup order payment should be accepted: %v", err)
	}
	f.clock.Set(time.Date(2026, time.August, 16, 10, 45, 0, 0, f.clock.Now().Location()))
	if _, err := f.service.MarkReadyForPickup(id); err != nil {
		t.Fatalf("paid order should become ready for pickup: %v", err)
	}
	f.clock.Set(time.Date(2026, time.August, 16, 11, 5, 0, 0, f.clock.Now().Location()))
	order, err := f.service.CompleteOrder(id)
	if err != nil {
		t.Fatalf("ready order should be completed after pickup: %v", err)
	}
	return order
}

func TestPickupOrderLifecycleRecordsDetailsPaymentAndStatus(t *testing.T) {
	fixture := newBusinessFixture(t)
	order := fixture.createOrder(t, "order-lifecycle", 500, 2, YogurtGreek)

	if order.CustomerID != fixture.customer.ID || order.StoreID != fixture.store.ID {
		t.Errorf("pickup order should retain its customer and store: %+v", order)
	}
	if order.Items[0].WeightGrams != 500 || order.Items[0].Yogurt != YogurtGreek || order.Items[0].AddOns[0].Quantity != 2 {
		t.Errorf("pickup order should retain weight, yogurt, and add-ons: %+v", order.Items)
	}
	if order.Pickup != fixture.pickupSlot() {
		t.Errorf("pickup order should retain its selected time slot: %+v", order.Pickup)
	}
	if order.Payment.Method != PaymentMethodMobilePay || order.Payment.Status != PaymentStatusPending {
		t.Errorf("pickup order should retain its selected pending payment: %+v", order.Payment)
	}
	if !order.Total.Equal(decimal.RequireFromString("50.00")) {
		t.Errorf("pickup order total should be 50.00, got %s", order.Total)
	}
	if order.Status != OrderStatusDraft || len(order.StatusHistory) != 1 {
		t.Errorf("new pickup order should be a recorded draft: status=%s history=%+v", order.Status, order.StatusHistory)
	}
	createdInventory := fixture.service.Inventory()
	if createdInventory.ProductGrams[fixture.product.ID] != 4500 || createdInventory.AddOnUnits[fixture.addOn.ID] != 98 {
		t.Errorf("new pickup order should reserve fruit and add-ons: %+v", createdInventory)
	}

	completed := fixture.completeOrder(t, order.ID)
	if completed.Status != OrderStatusCompleted {
		t.Errorf("picked-up order should be completed, got %s", completed.Status)
	}
	if completed.Payment.Status != PaymentStatusPaid || completed.Payment.Reference != "payment-001" {
		t.Errorf("completed order should retain its payment result: %+v", completed.Payment)
	}
	if completed.Payment.PaidAt == nil || completed.Payment.PaidAt.Hour() != 8 || completed.Payment.PaidAt.Minute() != 5 {
		t.Errorf("completed order should retain its payment time: %+v", completed.Payment.PaidAt)
	}
	wantStatuses := []OrderStatus{OrderStatusDraft, OrderStatusPaid, OrderStatusReadyForPickup, OrderStatusCompleted}
	gotStatuses := make([]OrderStatus, len(completed.StatusHistory))
	for index, record := range completed.StatusHistory {
		gotStatuses[index] = record.To
	}
	if !reflect.DeepEqual(gotStatuses, wantStatuses) {
		t.Errorf("pickup order should record its business statuses: got=%v want=%v", gotStatuses, wantStatuses)
	}
}

func TestDraftOrderUpdateChangesWeightAddOnsAndInventory(t *testing.T) {
	fixture := newBusinessFixture(t)
	order := fixture.createOrder(t, "order-editable", 400, 1, YogurtPlain)
	updatedSlot := fixture.pickupSlot()
	updatedSlot.Label = "11:30-12:00"
	updatedSlot.Start = updatedSlot.End
	updatedSlot.End = updatedSlot.Start.Add(30 * time.Minute)

	updated, err := fixture.service.UpdateOrder(order.ID, OrderUpdate{
		Items: []OrderItem{{
			ProductID:   fixture.product.ID,
			WeightGrams: 650,
			Yogurt:      YogurtGreek,
			AddOns:      []OrderAddOn{{AddOnID: fixture.addOn.ID, Quantity: 3}},
		}},
		Pickup: updatedSlot,
	})
	if err != nil {
		t.Fatalf("draft order changes should be accepted: %v", err)
	}
	if updated.Status != OrderStatusDraft || updated.Items[0].WeightGrams != 650 || updated.Items[0].Yogurt != YogurtGreek || updated.Items[0].AddOns[0].Quantity != 3 {
		t.Errorf("draft order should retain its updated content: %+v", updated)
	}
	if updated.Pickup != updatedSlot {
		t.Errorf("draft order should retain its updated pickup slot: %+v", updated.Pickup)
	}
	if !updated.Total.Equal(decimal.RequireFromString("64.50")) {
		t.Errorf("updated order total should be 64.50, got %s", updated.Total)
	}
	inventory := fixture.service.Inventory()
	if inventory.ProductGrams[fixture.product.ID] != 4350 || inventory.AddOnUnits[fixture.addOn.ID] != 97 {
		t.Errorf("updated draft should reserve its current fruit and add-ons: %+v", inventory)
	}
}

func TestCompletedOrderRejectsWeightAndAddOnChanges(t *testing.T) {
	fixture := newBusinessFixture(t)
	order := fixture.createOrder(t, "order-completed", 500, 1, YogurtPlain)
	fixture.completeOrder(t, order.ID)
	before, err := fixture.service.Order(order.ID)
	if err != nil {
		t.Fatalf("completed order should be available: %v", err)
	}
	beforeInventory := fixture.service.Inventory()
	beforeAdjustments := fixture.service.InventoryAdjustments(order.ID)

	_, updateErr := fixture.service.UpdateOrder(order.ID, OrderUpdate{
		Items: []OrderItem{{
			ProductID:   fixture.product.ID,
			WeightGrams: 750,
			Yogurt:      YogurtGreek,
			AddOns:      []OrderAddOn{{AddOnID: fixture.addOn.ID, Quantity: 3}},
		}},
		Pickup: fixture.pickupSlot(),
	})
	if !errors.Is(updateErr, ErrOrderNotEditable) {
		t.Errorf("completed order modification should be rejected, got %v", updateErr)
	}

	after, err := fixture.service.Order(order.ID)
	if err != nil {
		t.Fatalf("completed order should remain available: %v", err)
	}
	afterInventory := fixture.service.Inventory()
	afterAdjustments := fixture.service.InventoryAdjustments(order.ID)
	if after.Status != before.Status {
		t.Errorf("completed order status changed from %s to %s", before.Status, after.Status)
	}
	if !reflect.DeepEqual(after.Items, before.Items) {
		t.Errorf("completed order content changed from %+v to %+v", before.Items, after.Items)
	}
	if !reflect.DeepEqual(after.Payment, before.Payment) {
		t.Errorf("completed order payment changed from %+v to %+v", before.Payment, after.Payment)
	}
	if !reflect.DeepEqual(after.StatusHistory, before.StatusHistory) {
		t.Errorf("completed order status records changed from %+v to %+v", before.StatusHistory, after.StatusHistory)
	}
	if afterInventory.ProductGrams[fixture.product.ID] != beforeInventory.ProductGrams[fixture.product.ID] {
		t.Errorf("completed order fruit stock changed from %d to %d", beforeInventory.ProductGrams[fixture.product.ID], afterInventory.ProductGrams[fixture.product.ID])
	}
	if afterInventory.AddOnUnits[fixture.addOn.ID] != beforeInventory.AddOnUnits[fixture.addOn.ID] {
		t.Errorf("completed order add-on stock changed from %d to %d", beforeInventory.AddOnUnits[fixture.addOn.ID], afterInventory.AddOnUnits[fixture.addOn.ID])
	}
	if len(afterAdjustments) != len(beforeAdjustments) {
		t.Errorf("completed order inventory records changed from %d to %d", len(beforeAdjustments), len(afterAdjustments))
	}
}
