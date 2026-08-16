package fruitcut

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

var (
	plainYogurtPrice = decimal.RequireFromString("3.50")
	greekYogurtPrice = decimal.RequireFromString("5.00")
)

type OrderService struct {
	mu    sync.Mutex
	store *MemoryStore
	clock Clock
	ids   IDGenerator
}

func NewOrderService(store *MemoryStore, clock Clock, ids IDGenerator) *OrderService {
	if store == nil {
		store = NewMemoryStore()
	}
	if clock == nil {
		clock = SystemClock{}
	}
	if ids == nil {
		ids = NewSequenceIDGenerator(1)
	}
	return &OrderService{store: store, clock: clock, ids: ids}
}

func (s *OrderService) CreateCustomer(customer Customer) (Customer, error) {
	if strings.TrimSpace(customer.ID) == "" {
		customer.ID = s.ids.Next("customer")
	}
	if err := validateCustomer(customer); err != nil {
		return Customer{}, err
	}
	if err := s.store.createCustomer(customer); err != nil {
		return Customer{}, err
	}
	return customer, nil
}

func (s *OrderService) CreateStore(store Store) (Store, error) {
	if strings.TrimSpace(store.ID) == "" {
		store.ID = s.ids.Next("store")
	}
	if err := validateStore(store); err != nil {
		return Store{}, err
	}
	if err := s.store.createStore(store); err != nil {
		return Store{}, err
	}
	return store, nil
}

func (s *OrderService) CreateProduct(product Product) (Product, error) {
	if strings.TrimSpace(product.ID) == "" {
		product.ID = s.ids.Next("product")
	}
	if err := validateProduct(product); err != nil {
		return Product{}, err
	}
	if err := s.store.createProduct(product); err != nil {
		return Product{}, err
	}
	return product, nil
}

func (s *OrderService) CreateAddOn(addOn AddOn) (AddOn, error) {
	if strings.TrimSpace(addOn.ID) == "" {
		addOn.ID = s.ids.Next("addon")
	}
	if err := validateAddOn(addOn); err != nil {
		return AddOn{}, err
	}
	if err := s.store.createAddOn(addOn); err != nil {
		return AddOn{}, err
	}
	return addOn, nil
}

func (s *OrderService) CreateOrder(draft DraftOrder) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(draft.ID) == "" {
		draft.ID = s.ids.Next("order")
	}
	if strings.TrimSpace(draft.CustomerID) == "" || strings.TrimSpace(draft.StoreID) == "" {
		return Order{}, fmt.Errorf("%w: customer and store are required", ErrInvalidInput)
	}
	if err := validateItems(draft.Items); err != nil {
		return Order{}, err
	}
	if err := validatePickupSlot(draft.Pickup); err != nil {
		return Order{}, err
	}
	if err := validatePaymentMethod(draft.PaymentMethod); err != nil {
		return Order{}, err
	}
	if _, err := s.store.Customer(draft.CustomerID); err != nil {
		return Order{}, err
	}
	if _, err := s.store.Store(draft.StoreID); err != nil {
		return Order{}, err
	}
	if s.store.orderExists(draft.ID) {
		return Order{}, fmt.Errorf("%w: order %s", ErrDuplicateID, draft.ID)
	}

	total, productNeeds, addOnNeeds, err := s.priceAndRequirements(draft.Items)
	if err != nil {
		return Order{}, err
	}
	now := s.clock.Now()
	if err := s.store.applyInventoryDelta(draft.ID, productNeeds, addOnNeeds, now, "order created"); err != nil {
		return Order{}, err
	}
	order := Order{
		ID:         draft.ID,
		CustomerID: draft.CustomerID,
		StoreID:    draft.StoreID,
		Items:      cloneItems(draft.Items),
		Pickup:     draft.Pickup,
		Payment: Payment{
			Method: draft.PaymentMethod,
			Status: PaymentStatusPending,
			Amount: total,
		},
		Total:     total,
		Status:    OrderStatusDraft,
		CreatedAt: now,
		UpdatedAt: now,
		StatusHistory: []StatusRecord{{
			To:   OrderStatusDraft,
			At:   now,
			Note: "order created",
		}},
	}
	s.store.saveOrder(order)
	return cloneOrder(order), nil
}

func (s *OrderService) UpdateOrder(orderID string, update OrderUpdate) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	order, err := s.store.Order(orderID)
	if err != nil {
		return Order{}, err
	}
	if order.Status != OrderStatusDraft {
		return Order{}, ErrOrderNotEditable
	}
	if err := validateItems(update.Items); err != nil {
		return Order{}, err
	}
	if err := validatePickupSlot(update.Pickup); err != nil {
		return Order{}, err
	}

	total, newProducts, newAddOns, err := s.priceAndRequirements(update.Items)
	if err != nil {
		return Order{}, err
	}
	_, oldProducts, oldAddOns, err := s.priceAndRequirements(order.Items)
	if err != nil {
		return Order{}, err
	}
	productDelta := requirementDelta(newProducts, oldProducts)
	addOnDelta := requirementDelta(newAddOns, oldAddOns)
	now := s.clock.Now()
	if err := s.store.applyInventoryDelta(order.ID, productDelta, addOnDelta, now, "order updated"); err != nil {
		return Order{}, err
	}

	previousStatus := order.Status
	order.Items = cloneItems(update.Items)
	order.Pickup = update.Pickup
	order.Total = total
	order.Payment.Amount = total
	order.Payment.Status = PaymentStatusPending
	order.Payment.Reference = ""
	order.Payment.PaidAt = nil
	order.Status = OrderStatusDraft
	order.UpdatedAt = now
	if previousStatus != OrderStatusDraft {
		order.StatusHistory = append(order.StatusHistory, StatusRecord{
			From: previousStatus,
			To:   OrderStatusDraft,
			At:   now,
			Note: "order updated",
		})
	}
	s.store.saveOrder(order)
	return cloneOrder(order), nil
}

func (s *OrderService) PayOrder(orderID, reference string) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(reference) == "" {
		return Order{}, fmt.Errorf("%w: payment reference is required", ErrInvalidInput)
	}
	order, err := s.store.Order(orderID)
	if err != nil {
		return Order{}, err
	}
	if order.Status != OrderStatusDraft {
		return Order{}, ErrInvalidTransition
	}
	now := s.clock.Now()
	order.Payment.Status = PaymentStatusPaid
	order.Payment.Reference = reference
	order.Payment.PaidAt = &now
	transitionOrder(&order, OrderStatusPaid, now, "payment confirmed")
	s.store.saveOrder(order)
	return cloneOrder(order), nil
}

func (s *OrderService) MarkReadyForPickup(orderID string) (Order, error) {
	return s.transition(orderID, OrderStatusPaid, OrderStatusReadyForPickup, "order ready for pickup")
}

func (s *OrderService) CompleteOrder(orderID string) (Order, error) {
	return s.transition(orderID, OrderStatusReadyForPickup, OrderStatusCompleted, "order picked up")
}

func (s *OrderService) CancelOrder(orderID string) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, err := s.store.Order(orderID)
	if err != nil {
		return Order{}, err
	}
	if order.Status != OrderStatusDraft {
		return Order{}, ErrInvalidTransition
	}
	_, products, addOns, err := s.priceAndRequirements(order.Items)
	if err != nil {
		return Order{}, err
	}
	now := s.clock.Now()
	if err := s.store.applyInventoryDelta(order.ID, negateRequirements(products), negateRequirements(addOns), now, "order cancelled"); err != nil {
		return Order{}, err
	}
	transitionOrder(&order, OrderStatusCancelled, now, "order cancelled")
	s.store.saveOrder(order)
	return cloneOrder(order), nil
}

func (s *OrderService) Order(orderID string) (Order, error) {
	return s.store.Order(orderID)
}

func (s *OrderService) Customer(customerID string) (Customer, error) {
	return s.store.Customer(customerID)
}

func (s *OrderService) Store(storeID string) (Store, error) {
	return s.store.Store(storeID)
}

func (s *OrderService) Product(productID string) (Product, error) {
	return s.store.Product(productID)
}

func (s *OrderService) AddOn(addOnID string) (AddOn, error) {
	return s.store.AddOn(addOnID)
}

func (s *OrderService) Orders() []Order {
	return s.store.Orders()
}

func (s *OrderService) Inventory() InventorySnapshot {
	return s.store.Inventory()
}

func (s *OrderService) InventoryAdjustments(orderID string) []InventoryAdjustment {
	return s.store.InventoryAdjustments(orderID)
}

func (s *OrderService) transition(orderID string, from, to OrderStatus, note string) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, err := s.store.Order(orderID)
	if err != nil {
		return Order{}, err
	}
	if order.Status != from {
		return Order{}, ErrInvalidTransition
	}
	transitionOrder(&order, to, s.clock.Now(), note)
	s.store.saveOrder(order)
	return cloneOrder(order), nil
}

func (s *OrderService) priceAndRequirements(items []OrderItem) (decimal.Decimal, map[string]int, map[string]int, error) {
	total := decimal.Zero
	products := make(map[string]int)
	addOns := make(map[string]int)
	for _, item := range items {
		product, err := s.store.Product(item.ProductID)
		if err != nil {
			return decimal.Zero, nil, nil, err
		}
		products[item.ProductID] += item.WeightGrams
		total = total.Add(product.PricePerGram.Mul(decimal.NewFromInt(int64(item.WeightGrams))))
		total = total.Add(yogurtPrice(item.Yogurt))
		for _, line := range item.AddOns {
			addOn, err := s.store.AddOn(line.AddOnID)
			if err != nil {
				return decimal.Zero, nil, nil, err
			}
			addOns[line.AddOnID] += line.Quantity
			total = total.Add(addOn.Price.Mul(decimal.NewFromInt(int64(line.Quantity))))
		}
	}
	return total, products, addOns, nil
}

func yogurtPrice(option YogurtOption) decimal.Decimal {
	switch option {
	case YogurtPlain:
		return plainYogurtPrice
	case YogurtGreek:
		return greekYogurtPrice
	default:
		return decimal.Zero
	}
}

func transitionOrder(order *Order, to OrderStatus, at time.Time, note string) {
	from := order.Status
	order.Status = to
	order.UpdatedAt = at
	order.StatusHistory = append(order.StatusHistory, StatusRecord{
		From: from,
		To:   to,
		At:   at,
		Note: note,
	})
}

func requirementDelta(current, previous map[string]int) map[string]int {
	delta := make(map[string]int, len(current)+len(previous))
	for id, amount := range current {
		delta[id] += amount
	}
	for id, amount := range previous {
		delta[id] -= amount
	}
	return delta
}

func negateRequirements(values map[string]int) map[string]int {
	negated := make(map[string]int, len(values))
	for id, amount := range values {
		negated[id] = -amount
	}
	return negated
}
