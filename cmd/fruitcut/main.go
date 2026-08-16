package main

import (
	"fmt"
	"log"
	"time"

	fruitcut "example.com/fruitcut-orderbook"
	"github.com/shopspring/decimal"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	clock := fruitcut.SystemClock{}
	service := fruitcut.NewOrderService(fruitcut.NewMemoryStore(), clock, fruitcut.NewSequenceIDGenerator(1))

	customer, err := service.CreateCustomer(fruitcut.Customer{Name: "Lin", Phone: "13800000000"})
	if err != nil {
		return err
	}
	store, err := service.CreateStore(fruitcut.Store{Name: "Riverside Fruit Cut", Address: "18 Riverside Road"})
	if err != nil {
		return err
	}
	product, err := service.CreateProduct(fruitcut.Product{
		Name:         "Watermelon Cup",
		PricePerGram: decimal.RequireFromString("0.08"),
		StockGrams:   5000,
	})
	if err != nil {
		return err
	}
	addOn, err := service.CreateAddOn(fruitcut.AddOn{
		Name:       "Chia Seeds",
		Price:      decimal.RequireFromString("2.50"),
		StockUnits: 100,
	})
	if err != nil {
		return err
	}

	start := clock.Now().Add(90 * time.Minute).Truncate(30 * time.Minute)
	order, err := service.CreateOrder(fruitcut.DraftOrder{
		CustomerID: customer.ID,
		StoreID:    store.ID,
		Items: []fruitcut.OrderItem{{
			ProductID:   product.ID,
			WeightGrams: 500,
			Yogurt:      fruitcut.YogurtGreek,
			AddOns:      []fruitcut.OrderAddOn{{AddOnID: addOn.ID, Quantity: 2}},
		}},
		Pickup: fruitcut.PickupSlot{
			Label: start.Format("15:04") + "-" + start.Add(30*time.Minute).Format("15:04"),
			Start: start,
			End:   start.Add(30 * time.Minute),
		},
		PaymentMethod: fruitcut.PaymentMethodMobilePay,
	})
	if err != nil {
		return err
	}

	fmt.Printf("order=%s status=%s total=%s pickup=%s payment=%s\n", order.ID, order.Status, order.Total.StringFixed(2), order.Pickup.Label, order.Payment.Method)
	return nil
}
