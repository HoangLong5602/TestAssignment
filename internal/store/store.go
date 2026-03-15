package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	RoomStateWaiting = "waiting"
	RoomStatePlaying = "playing"
	RoomStateFinished = "finished"
)

const (
	KeyUserRoom          = "user:%s:room"
	KeyRoom              = "room:%s"
	KeyRoomMembers       = "room:%s:members"
	KeyRoomAnswered      = "room:%s:answered:%d"
	KeyRoomLeaderboard   = "room:%s:leaderboard"
	KeyRoomQuizStartedAt = "room:%s:quiz_started_at"
	ChannelLeaderboard       = "room:%s:leaderboard_updates"
	ChannelLeaderboardGlobal = "quiz:leaderboard_updates"
)

type Store struct {
	rdb *redis.Client
}

func New(addr, password string, db int) *Store {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	return &Store{rdb: rdb}
}

func (s *Store) Close() error {
	return s.rdb.Close()
}

func (s *Store) Client() *redis.Client {
	return s.rdb
}

func (s *Store) GetUserRoom(ctx context.Context, userID string) (string, error) {
	key := fmt.Sprintf(KeyUserRoom, userID)
	return s.rdb.Get(ctx, key).Result()
}

func (s *Store) SetUserRoom(ctx context.Context, userID, roomID string, ttl time.Duration) error {
	key := fmt.Sprintf(KeyUserRoom, userID)
	return s.rdb.Set(ctx, key, roomID, ttl).Err()
}

func (s *Store) DeleteUserRoom(ctx context.Context, userID string) error {
	key := fmt.Sprintf(KeyUserRoom, userID)
	return s.rdb.Del(ctx, key).Err()
}

// ClearAllUserRooms xóa mọi key user:{*}:room (dùng khi startup để sau restart user có thể join room khác).
func (s *Store) ClearAllUserRooms(ctx context.Context) error {
	const matchPattern = "user:*:room"
	var cursor uint64
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, matchPattern, 100).Result()
		if err != nil {
			return err
		}
		for _, k := range keys {
			_ = s.rdb.Del(ctx, k).Err()
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return nil
}

func (s *Store) GetRoomState(ctx context.Context, roomID string) (string, error) {
	key := fmt.Sprintf(KeyRoom, roomID)
	return s.rdb.HGet(ctx, key, "state").Result()
}

func (s *Store) RoomExists(ctx context.Context, roomID string) (bool, error) {
	key := fmt.Sprintf(KeyRoom, roomID)
	n, err := s.rdb.Exists(ctx, key).Result()
	return n > 0, err
}

func (s *Store) CreateRoom(ctx context.Context, roomID string, maxPlayers int) error {
	key := fmt.Sprintf(KeyRoom, roomID)
	return s.rdb.HSet(ctx, key,
		"state", RoomStateWaiting,
		"max_players", maxPlayers,
		"created_at", time.Now().Unix(),
	).Err()
}

func (s *Store) SetRoomState(ctx context.Context, roomID, state string) error {
	key := fmt.Sprintf(KeyRoom, roomID)
	return s.rdb.HSet(ctx, key, "state", state).Err()
}

func (s *Store) GetRoomMaxPlayers(ctx context.Context, roomID string) (int, error) {
	key := fmt.Sprintf(KeyRoom, roomID)
	return s.rdb.HGet(ctx, key, "max_players").Int()
}

func (s *Store) GetRoomCurrentQuizIndex(ctx context.Context, roomID string) (int, error) {
	key := fmt.Sprintf(KeyRoom, roomID)
	n, err := s.rdb.HGet(ctx, key, "current_quiz_index").Int()
	if err == redis.Nil {
		return -1, nil
	}
	return n, err
}

func (s *Store) SetRoomCurrentQuizIndex(ctx context.Context, roomID string, index int) error {
	key := fmt.Sprintf(KeyRoom, roomID)
	return s.rdb.HSet(ctx, key, "current_quiz_index", index).Err()
}

func (s *Store) GetAnsweredCount(ctx context.Context, roomID string, quizIndex int) (int64, error) {
	key := fmt.Sprintf(KeyRoomAnswered, roomID, quizIndex)
	return s.rdb.SCard(ctx, key).Result()
}

func (s *Store) AddRoomMember(ctx context.Context, roomID, userID string) error {
	key := fmt.Sprintf(KeyRoomMembers, roomID)
	return s.rdb.SAdd(ctx, key, userID).Err()
}

func (s *Store) RemoveRoomMember(ctx context.Context, roomID, userID string) error {
	key := fmt.Sprintf(KeyRoomMembers, roomID)
	return s.rdb.SRem(ctx, key, userID).Err()
}

func (s *Store) RoomMemberCount(ctx context.Context, roomID string) (int64, error) {
	key := fmt.Sprintf(KeyRoomMembers, roomID)
	return s.rdb.SCard(ctx, key).Result()
}

func (s *Store) HasAnswered(ctx context.Context, roomID string, quizIndex int, userID string) (bool, error) {
	key := fmt.Sprintf(KeyRoomAnswered, roomID, quizIndex)
	return s.rdb.SIsMember(ctx, key, userID).Result()
}

func (s *Store) MarkAnswered(ctx context.Context, roomID string, quizIndex int, userID string) (int64, error) {
	key := fmt.Sprintf(KeyRoomAnswered, roomID, quizIndex)
	return s.rdb.SAdd(ctx, key, userID).Result()
}

func (s *Store) SetQuizStartedAt(ctx context.Context, roomID string, quizIndex int) error {
	key := fmt.Sprintf(KeyRoomQuizStartedAt, roomID)
	return s.rdb.HSet(ctx, key, fmt.Sprintf("%d", quizIndex), time.Now().Unix()).Err()
}

func (s *Store) GetQuizStartedAt(ctx context.Context, roomID string, quizIndex int) (int64, error) {
	key := fmt.Sprintf(KeyRoomQuizStartedAt, roomID)
	return s.rdb.HGet(ctx, key, fmt.Sprintf("%d", quizIndex)).Int64()
}

func (s *Store) InitLeaderboardWithMembers(ctx context.Context, roomID string) error {
	membersKey := fmt.Sprintf(KeyRoomMembers, roomID)
	members, err := s.rdb.SMembers(ctx, membersKey).Result()
	if err != nil {
		return err
	}
	key := fmt.Sprintf(KeyRoomLeaderboard, roomID)
	for _, userID := range members {
		_ = s.rdb.ZAdd(ctx, key, redis.Z{Score: 0, Member: userID}).Err()
	}
	return nil
}

func (s *Store) IncrScore(ctx context.Context, roomID, userID string, delta float64) (float64, error) {
	key := fmt.Sprintf(KeyRoomLeaderboard, roomID)
	return s.rdb.ZIncrBy(ctx, key, delta, userID).Result()
}

func (s *Store) GetLeaderboard(ctx context.Context, roomID string, topN int64) ([]redis.Z, error) {
	key := fmt.Sprintf(KeyRoomLeaderboard, roomID)
	return s.rdb.ZRevRangeWithScores(ctx, key, 0, topN-1).Result()
}

func (s *Store) GetUserRank(ctx context.Context, roomID, userID string) (int64, error) {
	key := fmt.Sprintf(KeyRoomLeaderboard, roomID)
	rank, err := s.rdb.ZRevRank(ctx, key, userID).Result()
	if err == redis.Nil {
		return -1, nil
	}
	return rank + 1, err
}

func (s *Store) PublishLeaderboardUpdate(ctx context.Context, roomID string) error {
	return s.rdb.Publish(ctx, ChannelLeaderboardGlobal, roomID).Err()
}

func (s *Store) SubscribeLeaderboard(ctx context.Context, roomID string) *redis.PubSub {
	ch := fmt.Sprintf(ChannelLeaderboard, roomID)
	return s.rdb.Subscribe(ctx, ch)
}

func (s *Store) SubscribeLeaderboardGlobal(ctx context.Context) *redis.PubSub {
	return s.rdb.Subscribe(ctx, ChannelLeaderboardGlobal)
}

const maxQuizIndexCleanup = 50

func (s *Store) CleanupRoomAfterQuiz(ctx context.Context, roomID string) error {
	keys := []string{
		fmt.Sprintf(KeyRoomLeaderboard, roomID),
		fmt.Sprintf(KeyRoomQuizStartedAt, roomID),
	}
	for _, k := range keys {
		if err := s.rdb.Del(ctx, k).Err(); err != nil {
			return err
		}
	}
	for i := 0; i < maxQuizIndexCleanup; i++ {
		_ = s.rdb.Del(ctx, fmt.Sprintf(KeyRoomAnswered, roomID, i)).Err()
	}
	membersKey := fmt.Sprintf(KeyRoomMembers, roomID)
	members, err := s.rdb.SMembers(ctx, membersKey).Result()
	if err != nil {
		return err
	}
	for _, userID := range members {
		_ = s.DeleteUserRoom(ctx, userID)
	}
	if err := s.rdb.Del(ctx, membersKey).Err(); err != nil {
		return err
	}
	roomKey := fmt.Sprintf(KeyRoom, roomID)
	_ = s.rdb.HDel(ctx, roomKey, "current_quiz_index").Err()
	return s.SetRoomState(ctx, roomID, RoomStateWaiting)
}
