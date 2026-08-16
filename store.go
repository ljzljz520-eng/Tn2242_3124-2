package fruitcut

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

type InventorySnapshot struct {
	ProductGrams map[string]int
	AddOnUnits   map[string]int
}

type MemoryStore struct {
	mu                   sync.RWMutex
	customers            map[string]Customer
	stores               map[string]Store
	products             map[string]Product
	addOns               map[string]AddOn
	orders               map[string]Order
	inventoryAdjustments []InventoryAdjustment
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		customers: make(map[string]Customer),
		stores:    make(map[string]Store),
		products:  make(map[string]Product),
		addOns:    make(map[string]AddOn),
		orders:    make(map[string]Order),
	}
}

func (m *MemoryStore) createCustomer(customer Customer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.customers[customer.ID]; exists {
		return fmt.Errorf("%w: customer %s", ErrDuplicateID, customer.ID)
	}
	m.customers[customer.ID] = customer
	return nil
}

func (m *MemoryStore) createStore(store Store) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.stores[store.ID]; exists {
		return fmt.Errorf("%w: store %s", ErrDuplicateID, store.ID)
	}
	m.stores[store.ID] = store
	return nil
}

func (m *MemoryStore) createProduct(product Product) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.products[product.ID]; exists {
		return fmt.Errorf("%w: product %s", ErrDuplicateID, product.ID)
	}
	m.products[product.ID] = product
	return nil
}

func (m *MemoryStore) createAddOn(addOn AddOn) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.addOns[addOn.ID]; exists {
		return fmt.Errorf("%w: add-on %s", ErrDuplicateID, addOn.ID)
	}
	m.addOns[addOn.ID] = addOn
	return nil
}

func (m *MemoryStore) Customer(id string) (Customer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	customer, exists := m.customers[id]
	if !exists {
		return Customer{}, fmt.Errorf("%w: customer %s", ErrNotFound, id)
	}
	return customer, nil
}

func (m *MemoryStore) Store(id string) (Store, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	store, exists := m.stores[id]
	if !exists {
		return Store{}, fmt.Errorf("%w: store %s", ErrNotFound, id)
	}
	return store, nil
}

func (m *MemoryStore) Product(id string) (Product, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	product, exists := m.products[id]
	if !exists {
		return Product{}, fmt.Errorf("%w: product %s", ErrNotFound, id)
	}
	return product, nil
}

func (m *MemoryStore) AddOn(id string) (AddOn, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	addOn, exists := m.addOns[id]
	if !exists {
		return AddOn{}, fmt.Errorf("%w: add-on %s", ErrNotFound, id)
	}
	return addOn, nil
}

func (m *MemoryStore) Order(id string) (Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	order, exists := m.orders[id]
	if !exists {
		return Order{}, fmt.Errorf("%w: order %s", ErrNotFound, id)
	}
	return cloneOrder(order), nil
}

func (m *MemoryStore) orderExists(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.orders[id]
	return exists
}

func (m *MemoryStore) saveOrder(order Order) {
	m.mu.Lock()
	m.orders[order.ID] = cloneOrder(order)
	m.mu.Unlock()
}

func (m *MemoryStore) Orders() []Order {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.orders))
	for id := range m.orders {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	orders := make([]Order, 0, len(ids))
	for _, id := range ids {
		orders = append(orders, cloneOrder(m.orders[id]))
	}
	return orders
}

func (m *MemoryStore) Inventory() InventorySnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	snapshot := InventorySnapshot{
		ProductGrams: make(map[string]int, len(m.products)),
		AddOnUnits:   make(map[string]int, len(m.addOns)),
	}
	for id, product := range m.products {
		snapshot.ProductGrams[id] = product.StockGrams
	}
	for id, addOn := range m.addOns {
		snapshot.AddOnUnits[id] = addOn.StockUnits
	}
	return snapshot
}

func (m *MemoryStore) InventoryAdjustments(orderID string) []InventoryAdjustment {
	m.mu.RLock()
	defer m.mu.RUnlock()
	adjustments := make([]InventoryAdjustment, 0, len(m.inventoryAdjustments))
	for _, adjustment := range m.inventoryAdjustments {
		if orderID == "" || adjustment.OrderID == orderID {
			adjustments = append(adjustments, adjustment)
		}
	}
	return adjustments
}

func (m *MemoryStore) applyInventoryDelta(orderID string, productDelta, addOnDelta map[string]int, at time.Time, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, amount := range productDelta {
		product, exists := m.products[id]
		if !exists {
			return fmt.Errorf("%w: product %s", ErrNotFound, id)
		}
		if amount > product.StockGrams {
			return fmt.Errorf("%w: product %s", ErrInsufficientStock, id)
		}
	}
	for id, amount := range addOnDelta {
		addOn, exists := m.addOns[id]
		if !exists {
			return fmt.Errorf("%w: add-on %s", ErrNotFound, id)
		}
		if amount > addOn.StockUnits {
			return fmt.Errorf("%w: add-on %s", ErrInsufficientStock, id)
		}
	}

	productIDs := sortedMapKeys(productDelta)
	for _, id := range productIDs {
		amount := productDelta[id]
		if amount == 0 {
			continue
		}
		product := m.products[id]
		product.StockGrams -= amount
		m.products[id] = product
		m.inventoryAdjustments = append(m.inventoryAdjustments, InventoryAdjustment{
			OrderID:     orderID,
			ProductID:   id,
			StockChange: -amount,
			At:          at,
			Reason:      reason,
		})
	}

	addOnIDs := sortedMapKeys(addOnDelta)
	for _, id := range addOnIDs {
		amount := addOnDelta[id]
		if amount == 0 {
			continue
		}
		addOn := m.addOns[id]
		addOn.StockUnits -= amount
		m.addOns[id] = addOn
		m.inventoryAdjustments = append(m.inventoryAdjustments, InventoryAdjustment{
			OrderID:     orderID,
			AddOnID:     id,
			StockChange: -amount,
			At:          at,
			Reason:      reason,
		})
	}
	return nil
}

func sortedMapKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
