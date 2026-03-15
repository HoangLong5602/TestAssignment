# Quiz Room - Đa người chơi, điểm số & bảng xếp hạng thời gian thực

## Tổng quan

Ứng dụng quiz cho phép nhiều người chơi tham gia cùng một phòng (room), trả lời câu hỏi, tính điểm trên server và cập nhật bảng xếp hạng theo thời gian thực. Tech stack: **Golang**, **Redis**, **WebSocket** (Gorilla).

---

## Khung project

```
learning/
├── cmd/
│   └── quiz-server/
│       └── main.go          # Entry point, HTTP server, WebSocket, Redis, Hub
├── internal/
│   ├── config/
│   │   └── config.go        # Cấu hình (Redis, HTTP, max players, answer window, điểm)
│   ├── store/
│   │   └── store.go         # Redis: user-room, room state, idempotency, leaderboard
│   ├── hub/
│   │   ├── hub.go           # Hub quản lý rooms, register/unregister, inbound
│   │   ├── room.go          # Room, broadcast theo room
│   │   └── client.go        # WebSocket client, ReadPump/WritePump, SendJSON
│   ├── quiz/
│   │   ├── quiz.go          # Câu hỏi mặc định, ParseSubmitAnswer, ValidateAndScore
│   │   └── quiz_test.go
│   ├── handler/
│   │   └── handler.go       # HTTP + WS: tạo room, join, start/next/end quiz, WS connect
│   └── hub/
│       └── hub_test.go
├── docs/
│   └── QUIZ_ROOM.md         # File này
├── go.mod
└── go.sum
```

---

## Cách hoạt động

### 1. Nhiều người chơi trong một room

- **Room** được tạo qua API `POST /room`, server trả về `room_id` (UUID).
- User **join room** qua `POST /room/join` với `user_id` và `room_id`.
- **Validate khi join:**
  - User chỉ được ở **một room** tại một thời điểm: Redis key `user:{userId}:room` → nếu đã có và khác `room_id` thì trả lỗi `user_already_in_another_room`.
  - Room phải tồn tại, `state = waiting`, chưa đủ số người (so với `max_players`).
- Kết nối **WebSocket** qua `GET /ws?user_id=...&room_id=...`: chỉ chấp nhận nếu user đã join room (đã có `user:{userId}:room` = `room_id`).
- Khi client **ngắt kết nối**, handler gọi `RemoveRoomMember` và `DeleteUserRoom` để user có thể tham gia room khác sau này.

### 2. Cập nhật điểm số theo thời gian thực

- **Tính điểm trên server** để tránh gian lận.
- **Thời gian trả lời:** server kiểm tra `quiz_started_at` (Redis) + `ANSWER_WINDOW_SEC`; nếu gửi sau khi hết giờ thì từ chối (`answer_time_expired`).
- **Một câu một lần trả lời:** dùng idempotency bằng Redis set `room:{roomId}:answered:{quizIndex}`; `SADD` user vào set, nếu đã có (trả về 0) thì từ chối (`already_answered`).
- **Cập nhật điểm:** khi đáp án đúng, gọi `ZINCRBY` trên key `room:{roomId}:leaderboard` (sorted set) → đảm bảo atomic, tránh race/lost update.
- Sau khi cộng điểm, server **publish** vào channel Redis `quiz:leaderboard_updates` với payload là `roomID`.

### 3. Bảng xếp hạng theo thời gian thực

- Một goroutine **subscribe** channel `quiz:leaderboard_updates`.
- Mỗi khi nhận message (roomID), lấy bảng xếp hạng bằng `ZREVRANGE ... WITHSCORES` (top N), marshal JSON và **broadcast** tới tất cả client trong room qua WebSocket.
- Khi **kết thúc quiz**, API `POST /room/end` dùng `ZREVRANGE` để trả top BXH và (nếu cần) `ZREVRANK` cho từng user; sau đó cleanup room và trả user về trạng thái sẵn sàng tham gia room tiếp theo.

---

## API & WebSocket

| Method / Loại | Endpoint / URL | Mô tả |
|---------------|----------------|-------|
| POST | `/room` | Tạo room mới, trả về `room_id` |
| POST | `/room/join` | Body: `{"user_id","room_id"}`. Join room (validate 1 room/user, state, số lượng). |
| GET | `/ws?user_id=...&room_id=...` | Nâng cấp WebSocket; user phải đã join room. |
| POST | `/room/start?room_id=...` | Bắt đầu quiz (5 câu). Tự chuyển câu sau 20s hoặc khi mọi người trả lời xong; hết 5 câu thì tự kết thúc và broadcast `quiz_end`. |
| POST | `/room/next` | Body: `{"quiz_index":N}`. Chuyển câu thủ công (tùy chọn). |
| POST | `/room/end?room_id=...` | Kết thúc quiz thủ công, trả BXH, cleanup. |

**WebSocket message từ client (submit đáp án):**

```json
{"type":"submit_answer","quiz_index":0,"answer_index":1}
```

**WebSocket message từ server:**

- `question`: câu hỏi hiện tại (khi start/next hoặc tự chuyển câu).
- `answer_result`: kết quả submit (accepted, reason, score).
- `leaderboard_update`: cập nhật BXH theo thời gian thực.
- `quiz_end`: quiz kết thúc (tự động hoặc thủ công), kèm `leaderboard` – client nên hiển thị popup BXH.
- `leaderboard_update`: danh sách top (rank, user, score).

---

## Redis – Key & Channel

| Key / Channel | Kiểu | Mục đích |
|---------------|------|----------|
| `user:{userId}:room` | string | Room hiện tại của user (TTL 24h). |
| `room:{roomId}` | hash | state, max_players, created_at, current_quiz_index. |
| `room:{roomId}:members` | set | Danh sách user trong room. |
| `room:{roomId}:answered:{quizIndex}` | set | User đã trả lời câu đó (idempotency). |
| `room:{roomId}:leaderboard` | sorted set | Điểm (score) → BXH. |
| `room:{roomId}:quiz_started_at` | hash | Thời điểm bắt đầu từng câu (field = quiz index). |
| `quiz:leaderboard_updates` | channel | Pub/Sub: payload = roomID khi có cập nhật điểm. |

---

## Tại sao user có thể join room khác sau khi kết thúc room hoặc restart?

- **Ràng buộc:** Mỗi user chỉ được ở **một room** tại một thời điểm; server kiểm tra qua Redis key `user:{userId}:room`.
- **Khi room kết thúc (quiz_end hoặc POST /room/end):** Server gọi `CleanupRoomAfterQuiz`: xóa leaderboard, answered, members, và **xóa `user:{userId}:room`** cho từng thành viên trong room. Sau đó set room state về `waiting`. Vì key “user đang ở room nào” đã bị xóa, lần join tiếp theo sẽ không thấy user trong room cũ → user có thể join room khác bình thường.
- **Khi restart service:** Toàn bộ kết nối WebSocket và state in-memory (Hub, timer) mất, nhưng **Redis vẫn giữ** các key `user:{userId}:room`. Nếu không xử lý, user vẫn bị coi là “đang trong room cũ” và sẽ nhận lỗi `user_already_in_another_room` khi join room khác. Để trạng thái “trở lại bình thường” sau restart, server khi khởi động có thể **xóa toàn bộ key user-room** (xem mục “Reset user-room khi startup” bên dưới), coi mọi binding cũ là stale vì connection đã mất.

---

## Validate nghiệp vụ (logic)

- Chỉ cho phép user ở **một room** tại một thời điểm (Redis `user:{userId}:room`).
- Join room: room tồn tại, `state = waiting`, chưa đủ `max_players`.
- WebSocket: chỉ connect nếu đã join room (GetUserRoom == room_id).
- Submit answer: room `playing`, trong thời gian trả lời, chưa trả lời câu đó (SADD idempotency), sau đó ZINCRBY và publish leaderboard.

---

## Cách chạy & sử dụng

### Yêu cầu

- Go 1.23+
- Redis (mặc định `localhost:6379`)

### Biến môi trường (tùy chọn)

- `HTTP_ADDR`: địa chỉ HTTP (mặc định `:8080`)
- `REDIS_ADDR`, `REDIS_PASSWORD`, `REDIS_DB`
- `MAX_PLAYERS_PER_ROOM`: số người tối đa mỗi room (mặc định 10)
- `ANSWER_WINDOW_SEC`: thời gian cho phép trả lời mỗi câu (giây)
- `QUESTION_TIMEOUT_SEC`: tự chuyển sang câu sau sau N giây (mặc định 20)
- `POINTS_PER_CORRECT`: điểm mỗi câu đúng
- `CLEAR_USER_ROOM_ON_STARTUP`: set `1` hoặc `true` để khi khởi động xóa hết binding user↔room trong Redis, giúp sau restart user có thể join room khác ngay (mặc định không xóa)

### Build & chạy

**Cách 1 – Docker (Redis + app):**

```bash
cd learning
docker compose up --build
# Ứng dụng: http://localhost:8080, Redis: localhost:6379
```

**Cách 2 – Local:**

```bash
cd learning
go build -o quiz-server.exe ./cmd/quiz-server
# Chạy Redis (Docker: docker run -p 6379:6379 redis:7-alpine hoặc local)
./quiz-server.exe
```

### Luồng sử dụng nhanh

1. **Tạo room:** `POST /room` → nhận `room_id`.
2. **Nhiều user join:** `POST /room/join` với cùng `room_id`, mỗi user một `user_id`.
3. **Kết nối WebSocket:** `GET /ws?user_id=...&room_id=...` (mỗi client một connection).
4. **Bắt đầu quiz:** `POST /room/start?room_id=...` → client nhận `question` (câu 0). Có 5 câu hỏi.
5. **Tự chuyển câu:** Sau 20s (cấu hình `QUESTION_TIMEOUT_SEC`) hoặc khi **mọi người trong room đã trả lời** → server tự broadcast câu tiếp. Hết 5 câu → tự kết thúc và gửi `quiz_end` kèm BXH.
6. **Client gửi đáp án:** `{"type":"submit_answer","quiz_index":0,"answer_index":1}`. Server trả `answer_result` và `leaderboard_update`.
7. **Kết thúc (tùy chọn):** `POST /room/end?room_id=...` để kết thúc thủ công; sau đó cleanup, user có thể join room khác.
8. **Popup UX:** Khi nhận `quiz_end`, client (ví dụ trang test `/test/`) hiển thị popup bảng xếp hạng.

---

## Cách test (không bắt buộc FE riêng)

### Cách 1: Trang test client có sẵn (nhanh nhất)

Server có sẵn trang HTML để test toàn bộ luồng (tạo room, join, WebSocket, gửi đáp án, BXH).

1. Chạy server (và Redis).
2. Mở trình duyệt: **http://localhost:8080/test/** (hoặc **http://localhost:8080/test**).
3. Trên trang: **Tạo room** → **Join room** (nhập User ID, ví dụ `user1`) → **Kết nối WebSocket**.
4. (Tab khác hoặc user khác): cùng Room ID, User ID khác (ví dụ `user2`) → Join → Kết nối WebSocket.
5. Dùng **Bắt đầu quiz** → mọi client nhận câu hỏi → bấm đáp án để gửi → xem **Log & BXH**.
6. **Câu tiếp theo** / **Kết thúc quiz** để điều khiển từ host.

**Không cần viết FE riêng** – trang này đủ để kiểm thử API + WebSocket.

### Cách 2: Chỉ dùng curl + WebSocket (không mở browser)

**HTTP (curl):**

```bash
# Tạo room
curl -X POST http://localhost:8080/room

# Join (thay ROOM_ID, user1 / user2)
curl -X POST http://localhost:8080/room/join -H "Content-Type: application/json" -d "{\"user_id\":\"user1\",\"room_id\":\"ROOM_ID\"}"

# Bắt đầu quiz
curl -X POST "http://localhost:8080/room/start?room_id=ROOM_ID"

# Kết thúc
curl -X POST "http://localhost:8080/room/end?room_id=ROOM_ID"
```

**WebSocket:** dùng công cụ như **wscat** (npm: `npx wscat -c "ws://localhost:8080/ws?user_id=user1&room_id=ROOM_ID"`). Sau khi kết nối, gửi JSON:

```json
{"type":"submit_answer","quiz_index":0,"answer_index":1}
```

### Tóm lại

| Cách | Cần FE? | Ghi chú |
|------|--------|--------|
| Trang **/test/** | Không | Trang HTML có sẵn, nhúng trong binary, mở browser là test được. |
| curl + wscat | Không | Test từng API và WS thủ công. |
| FE riêng (React, Vue…) | Có | Chỉ cần khi muốn làm sản phẩm UI riêng; logic vẫn dùng cùng API/WS. |

---

## Test (unit test)

```bash
go test ./internal/quiz/... ./internal/hub/... -v
```

- **quiz:** ParseSubmitAnswer (JSON, type, invalid), GetDefaultQuestions, roundtrip JSON.
- **hub:** NewHub, newRoom, add/remove client, Register/Unregister với Hub đang chạy.

Test không cần Redis; test tích hợp đầy đủ (HTTP + WS + Redis) có thể bổ sung với Redis thật hoặc mock.

---

## Hướng xử lý (tóm tắt)

| Yêu cầu | Cách làm |
|--------|----------|
| Nhiều người một room | WebSocket Hub + Room (map roomID → Room, mỗi Room có map client). |
| User chỉ 1 room | Redis `user:{userId}:room`; join/WS kiểm tra và set/xóa khi leave. |
| Room chưa bắt đầu, chưa đủ người | state = waiting, so sánh SCARD(members) với max_players. |
| Trả user về sẵn sàng sau quiz | CleanupRoomAfterQuiz: xóa leaderboard, answered, quiz_started_at, members, xóa user:room cho từng member, set state = waiting. |
| Điểm trên server, đúng thời gian | So sánh thời gian gửi với quiz_started_at + ANSWER_WINDOW_SEC. |
| Một câu một lần trả lời | Redis set `room:{roomId}:answered:{quizIndex}`, SADD; nếu 0 thì từ chối. |
| Cập nhật điểm atomic | ZINCRBY trên sorted set leaderboard. |
| BXH thời gian thực | Publish roomID vào `quiz:leaderboard_updates`; subscriber lấy ZREVRANGE rồi broadcast qua WebSocket cho room. |
| Kết thúc quiz | ZREVRANGE top, ZREVRANK nếu cần; sau đó cleanup và set state waiting. |

---

## Scale ngang (nhiều server)

- **User–room & state:** đã dùng Redis nên nhiều instance app có thể dùng chung dữ liệu.
- **Leaderboard:** Redis sorted set + pub/sub đảm bảo mọi server nhận cập nhật qua channel `quiz:leaderboard_updates`; mỗi server chỉ broadcast BXH tới các client WebSocket đang kết nối với chính nó (sticky session hoặc route theo room/user nếu cần).
