# 果切店自取订单库

这是一个 Go 1.22 单模块项目，使用内存存储管理客户、门店、按克计价的水果商品、酸奶选项、加料库存、自取时段、支付和订单状态记录。金额计算使用固定版本的 `github.com/shopspring/decimal`，避免重量计价出现浮点误差。

## 运行入口

从项目根目录启动命令行示例：

```bash
go run ./cmd/fruitcut
```

示例会创建客户、门店、商品、加料和一笔待支付自取订单，并输出订单号、状态、金额、取货时段和支付方式。

## 业务流程

订单从 `draft` 开始，支付后进入 `paid`，备货完成进入 `ready_for_pickup`，客户取货后进入 `completed`。创建和编辑草稿时，商品库存按克、加料库存按份调整；取消未支付订单会归还已预留库存。每次状态变化都保留时间和业务说明。

主要入口位于根包 `example.com/fruitcut-orderbook`：

- `NewOrderService` 创建订单服务
- `CreateCustomer`、`CreateStore`、`CreateProduct`、`CreateAddOn` 管理基础资料
- `CreateOrder`、`UpdateOrder`、`PayOrder`、`MarkReadyForPickup`、`CompleteOrder`、`CancelOrder` 管理订单流程
- `Order`、`Orders`、`Inventory`、`InventoryAdjustments` 查询订单和库存

## 测试与构建

完整业务链路测试命令：

```bash
go test -count=1 ./...
```

项目故意保留一处完成订单二次修改缺陷。触发条件是已完成订单再次提交重量和加料修改；当前错误表现是订单重新变为草稿、内容被改写，商品和加料库存再次调整。业务要求是拒绝该请求，并保持完成时的订单、支付、状态记录与库存不变。因此当前命令会稳定由 `TestCompletedOrderRejectsWeightAndAddOnChanges` 报告失败，其余业务链路测试通过。

纯 Go 构建验证：

```bash
CGO_ENABLED=0 go build ./...
```

测试仅使用内存存储、固定时钟和顺序 ID，不依赖网络、外部数据库、真实时间、随机数或操作系统特定行为。
