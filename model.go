package fruitcut

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type OrderStatus string

const (
	OrderStatusDraft          OrderStatus = "draft"
	OrderStatusPaid           OrderStatus = "paid"
	OrderStatusReadyForPickup OrderStatus = "ready_for_pickup"
	OrderStatusCompleted      OrderStatus = "completed"
	OrderStatusCancelled      OrderStatus = "cancelled"
)

type PaymentMethod string

const (
	PaymentMethodCash      PaymentMethod = "cash"
	PaymentMethodCard      PaymentMethod = "card"
	PaymentMethodMobilePay PaymentMethod = "mobile_pay"
)

type PaymentStatus string

const (
	PaymentStatusPending PaymentStatus = "pending"
	PaymentStatusPaid    PaymentStatus = "paid"
)

type YogurtOption string

const (
	YogurtNone  YogurtOption = "none"
	YogurtPlain YogurtOption = "plain"
	YogurtGreek YogurtOption = "greek"
)

var (
	ErrNotFound          = errors.New("requested record was not found")
	ErrInvalidInput      = errors.New("order data is invalid")
	ErrInvalidTransition = errors.New("order status transition is not allowed")
	ErrOrderNotEditable  = errors.New("order cannot be edited in its current status")
	ErrInsufficientStock = errors.New("requested quantity exceeds available stock")
	ErrDuplicateID       = errors.New("record identifier already exists")
)

type Customer struct {
	ID    string
	Name  string
	Phone string
}

type Store struct {
	ID      string
	Name    string
	Address string
}

type Product struct {
	ID           string
	Name         string
	PricePerGram decimal.Decimal
	StockGrams   int
}

type AddOn struct {
	ID         string
	Name       string
	Price      decimal.Decimal
	StockUnits int
}

type PickupSlot struct {
	Label string
	Start time.Time
	End   time.Time
}

type OrderAddOn struct {
	AddOnID  string
	Quantity int
}

type OrderItem struct {
	ProductID   string
	WeightGrams int
	Yogurt      YogurtOption
	AddOns      []OrderAddOn
}

type Payment struct {
	Method    PaymentMethod
	Status    PaymentStatus
	Amount    decimal.Decimal
	Reference string
	PaidAt    *time.Time
}

type StatusRecord struct {
	From OrderStatus
	To   OrderStatus
	At   time.Time
	Note string
}

type Order struct {
	ID            string
	CustomerID    string
	StoreID       string
	Items         []OrderItem
	Pickup        PickupSlot
	Payment       Payment
	Total         decimal.Decimal
	Status        OrderStatus
	StatusHistory []StatusRecord
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type DraftOrder struct {
	ID            string
	CustomerID    string
	StoreID       string
	Items         []OrderItem
	Pickup        PickupSlot
	PaymentMethod PaymentMethod
}

type OrderUpdate struct {
	Items  []OrderItem
	Pickup PickupSlot
}

type InventoryAdjustment struct {
	OrderID     string
	ProductID   string
	AddOnID     string
	StockChange int
	At          time.Time
	Reason      string
}

func (s OrderStatus) String() string {
	return string(s)
}

func (m PaymentMethod) String() string {
	return string(m)
}

func (y YogurtOption) String() string {
	return string(y)
}

func validateCustomer(customer Customer) error {
	if strings.TrimSpace(customer.ID) == "" || strings.TrimSpace(customer.Name) == "" || strings.TrimSpace(customer.Phone) == "" {
		return fmt.Errorf("%w: customer fields are required", ErrInvalidInput)
	}
	return nil
}

func validateStore(store Store) error {
	if strings.TrimSpace(store.ID) == "" || strings.TrimSpace(store.Name) == "" || strings.TrimSpace(store.Address) == "" {
		return fmt.Errorf("%w: store fields are required", ErrInvalidInput)
	}
	return nil
}

func validateProduct(product Product) error {
	if strings.TrimSpace(product.ID) == "" || strings.TrimSpace(product.Name) == "" || product.PricePerGram.IsNegative() || product.PricePerGram.IsZero() || product.StockGrams < 0 {
		return fmt.Errorf("%w: product fields are invalid", ErrInvalidInput)
	}
	return nil
}

func validateAddOn(addOn AddOn) error {
	if strings.TrimSpace(addOn.ID) == "" || strings.TrimSpace(addOn.Name) == "" || addOn.Price.IsNegative() || addOn.StockUnits < 0 {
		return fmt.Errorf("%w: add-on fields are invalid", ErrInvalidInput)
	}
	return nil
}

func validatePaymentMethod(method PaymentMethod) error {
	switch method {
	case PaymentMethodCash, PaymentMethodCard, PaymentMethodMobilePay:
		return nil
	default:
		return fmt.Errorf("%w: payment method is invalid", ErrInvalidInput)
	}
}

func validatePickupSlot(slot PickupSlot) error {
	if strings.TrimSpace(slot.Label) == "" || slot.Start.IsZero() || slot.End.IsZero() || !slot.Start.Before(slot.End) {
		return fmt.Errorf("%w: pickup slot is invalid", ErrInvalidInput)
	}
	return nil
}

func validateItem(item OrderItem) error {
	if strings.TrimSpace(item.ProductID) == "" || item.WeightGrams <= 0 {
		return fmt.Errorf("%w: item weight is invalid", ErrInvalidInput)
	}
	switch item.Yogurt {
	case YogurtNone, YogurtPlain, YogurtGreek:
	default:
		return fmt.Errorf("%w: yogurt option is invalid", ErrInvalidInput)
	}
	for _, addOn := range item.AddOns {
		if strings.TrimSpace(addOn.AddOnID) == "" || addOn.Quantity <= 0 {
			return fmt.Errorf("%w: add-on quantity is invalid", ErrInvalidInput)
		}
	}
	return nil
}

func validateItems(items []OrderItem) error {
	if len(items) == 0 {
		return fmt.Errorf("%w: at least one item is required", ErrInvalidInput)
	}
	for _, item := range items {
		if err := validateItem(item); err != nil {
			return err
		}
	}
	return nil
}

func cloneItems(items []OrderItem) []OrderItem {
	if items == nil {
		return nil
	}
	cloned := make([]OrderItem, len(items))
	for i, item := range items {
		cloned[i] = item
		if item.AddOns != nil {
			cloned[i].AddOns = append([]OrderAddOn(nil), item.AddOns...)
		}
	}
	return cloned
}

func cloneOrder(order Order) Order {
	order.Items = cloneItems(order.Items)
	if order.StatusHistory != nil {
		order.StatusHistory = append([]StatusRecord(nil), order.StatusHistory...)
	}
	if order.Payment.PaidAt != nil {
		paidAt := *order.Payment.PaidAt
		order.Payment.PaidAt = &paidAt
	}
	return order
}
