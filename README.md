# olist-marketplace

Dự án học microservices (Go) + BMAD, dùng dataset [Brazilian E-Commerce by Olist](https://www.kaggle.com/datasets/olistbr/brazilian-ecommerce).

## Cấu trúc

| Thư mục | Vai trò |
|---|---|
| `services/` | 7 service, mỗi service là một Go module riêng: `api-gateway`, `identity-service`, `catalog-service`, `order-service`, `payment-service`, `shipping-simulator`, `analytics-service`. Service có dữ liệu dùng layout hexagonal (`cmd/server`, `internal/{domain,service,repository,transport}`, `migrations/`) |
| `proto/` | Hợp đồng gRPC `proto/<service>/v1`, kiểm tra bằng `buf`; code sinh ra nằm ở `proto/gen/go` |
| `contracts/events/v1/` | Danh mục event và command Kafka (nguồn duy nhất) |
| `pkg/` | Thư viện dùng chung: config, dbmigrate, bootstrap, logger, tracing, eventbus, outbox, inbox |
| `deployments/` | `docker-compose.yml` chạy local, init script Postgres (mỗi service một DB + user), `k8s/` cho tuần 8 |
| `tests/scaffold/` | Test kiểm tra cấu trúc repo khớp kiến trúc |
| `tools/olist-importer/` | Script nạp CSV Kaggle |
| `data/` | CSV Olist (không commit) |
| `docs/` | Tài liệu BMAD: brief, PRD, architecture, stories |

## Lệnh thường dùng

Cần Go 1.27 và Docker.

```bash
make build                # build mọi service vào ./bin
make test                 # test mọi service và module dùng chung
make proto                # buf generate + lint + breaking (so với develop)
make up                   # docker compose: web, 7 service + Postgres, Kafka, Jaeger, Prometheus, Grafana, kafka-ui
make import               # nạp Category, Seller, Product vào catalog và 41 account demo vào identity (chạy lại không tạo trùng)
make check-db-isolation   # kiểm tra mỗi DB user chỉ vào được DB của mình
make dlq-check            # lỗi nếu còn message nằm trong topic *.dlq chưa redrive
make dlq-redrive TOPIC=order.events   # đưa order.events.dlq về lại order.events
make web-test             # lint, test và build web app (Node 24)
make down
```

Port mở ra host (chỉ trên `127.0.0.1`): web app `5173` (mở http://127.0.0.1:5173), api-gateway `8080`, Jaeger `16686`, Prometheus `9090`, Grafana `3000`, kafka-ui `8081`.

`make import` cần dataset Olist (Kaggle) trong `data/`. Importer chạy trong container trên mạng compose và chỉ gọi admin gRPC của catalog-service và identity-service. Biến môi trường của importer: `DATA_DIR` (mặc định `/data`), `CATALOG_ADDR` (mặc định `catalog-service:9090`), `IDENTITY_ADDR` (mặc định `identity-service:9090`), `DEMO_PASSWORD` (mặc định `olist-demo-2026`). Test `TestLoadRealDataset` chỉ chạy khi đặt `OLIST_DATA_DIR` trỏ tới thư mục CSV.

Checkout: Customer gọi `POST /api/checkout` với `{"product_ids": [...], "checkout_idempotency_key": "..."}`; order-service lấy snapshot giá, phí ship, Seller và Category từ catalog (gRPC, timeout `CATALOG_TIMEOUT`, mặc định `2s`) rồi tạo mỗi Seller một Order `created` cùng event `OrderCreated` trong một transaction. Gửi lại cùng key với cùng Product trả về các Order cũ; cùng key với Product khác trả 409; catalog không trả lời kịp trả 503 và không ghi gì.

Event lên Kafka chỉ đi qua outbox: use case gọi `outbox.Write(ctx, tx, event, meta)` trong transaction của mình, và relay của service (`app.Service{Outbox: true}`) publish các dòng chưa gửi theo thứ tự `seq`. Chỉ loại event có trong `contracts/events/v1` mới ghi được; JSON Schema trong `contracts/events/v1/schema` được sinh từ struct Go (`cd contracts && go generate ./...`, có test báo khi lệch). Biến môi trường: `OUTBOX_POLL_INTERVAL` (mặc định `200ms`, trong khoảng `10ms`–`200ms`). Metric `outbox_unsent_rows` đếm số dòng chưa gửi. Consumer dùng `pkg/inbox` (`app.Service{Consumers: …}`): mỗi event được áp dụng cùng dòng `processed_events` trong một transaction, nên nhận lại cùng `event_id` chỉ là `duplicate`. Handler trả `inbox.Reject(reason)` khi event sai về nghiệp vụ (ghi lại, ack, không thử lại); lỗi khác được thử lại tại chỗ (100ms nhân đôi, tối đa 5s), quá `INBOX_MAX_ATTEMPTS` lần (mặc định 5) thì message sang `<topic>.dlq` và partition đi tiếp. Metric: `inbox_messages_total{consumer,topic,outcome}`, `inbox_dlq_parked_total{topic}`, `inbox_dlq_depth{topic}`. Message trong DLQ mang header `dlq-reason`, `dlq-attempts`, `dlq-consumer`; redrive gửi lại cho mọi consumer group của topic và message đến sau các event mới hơn của cùng Order, nên xem lý do trước khi redrive. Test tích hợp outbox và inbox cần thêm `OUTBOX_TEST_KAFKA` (broker Kafka) bên cạnh `DBMIGRATE_TEST_DSN`.

Init script Postgres chỉ chạy khi volume dữ liệu được tạo lần đầu. Đổi mật khẩu DB (`*_DB_PASSWORD`) hoặc thêm database thì phải xoá volume: `docker compose -f deployments/docker-compose.yml down -v`.

## Account demo

`make import` tạo 41 account demo, tất cả dùng chung mật khẩu `olist-demo-2026` (đổi bằng `DEMO_PASSWORD=... make import`; chạy lại sẽ đặt mật khẩu mới cho cả 41 account). Email trong domain `@demo.olist.local` chỉ dành cho account demo, không đăng ký được.

- **Admin:** `admin@demo.olist.local`

**Customer** — 20 `customer_unique_id` có nhiều Order nhất (id của account là `customer_unique_id`):

| Email | Số Order |
|---|---|
| `customer-8d50f5ea@demo.olist.local` | 17 |
| `customer-3e43e610@demo.olist.local` | 9 |
| `customer-1b6c7548@demo.olist.local` | 7 |
| `customer-6469f99c@demo.olist.local` | 7 |
| `customer-ca77025e@demo.olist.local` | 7 |
| `customer-12f5d6e1@demo.olist.local` | 6 |
| `customer-47c1a303@demo.olist.local` | 6 |
| `customer-63cfc61c@demo.olist.local` | 6 |
| `customer-dc813062@demo.olist.local` | 6 |
| `customer-de34b161@demo.olist.local` | 6 |
| `customer-f0e310a6@demo.olist.local` | 6 |
| `customer-35ecdf68@demo.olist.local` | 5 |
| `customer-394ac4de@demo.olist.local` | 5 |
| `customer-4e65032f@demo.olist.local` | 5 |
| `customer-56c8638e@demo.olist.local` | 5 |
| `customer-5e8f38a9@demo.olist.local` | 5 |
| `customer-74cb1ad7@demo.olist.local` | 5 |
| `customer-b4e4f24d@demo.olist.local` | 5 |
| `customer-fe81bb32@demo.olist.local` | 5 |
| `customer-083ca1aa@demo.olist.local` | 4 |

**Seller** — 20 `seller_id` có nhiều Order Item nhất (id của account là `seller_id`):

| Email | Số Order Item |
|---|---|
| `seller-6560211a@demo.olist.local` | 2033 |
| `seller-4a3ca931@demo.olist.local` | 1987 |
| `seller-1f50f920@demo.olist.local` | 1931 |
| `seller-cc419e06@demo.olist.local` | 1775 |
| `seller-da8622b1@demo.olist.local` | 1551 |
| `seller-955fee92@demo.olist.local` | 1499 |
| `seller-1025f0e2@demo.olist.local` | 1428 |
| `seller-7c67e144@demo.olist.local` | 1364 |
| `seller-ea8482cd@demo.olist.local` | 1203 |
| `seller-7a67c85e@demo.olist.local` | 1171 |
| `seller-4869f7a5@demo.olist.local` | 1156 |
| `seller-3d871de0@demo.olist.local` | 1147 |
| `seller-8b321bb6@demo.olist.local` | 1018 |
| `seller-cca3071e@demo.olist.local` | 830 |
| `seller-620c87c1@demo.olist.local` | 798 |
| `seller-a1043baf@demo.olist.local` | 770 |
| `seller-e9779976@demo.olist.local` | 750 |
| `seller-f8db351d@demo.olist.local` | 724 |
| `seller-d2374cbc@demo.olist.local` | 631 |
| `seller-391fc663@demo.olist.local` | 613 |
