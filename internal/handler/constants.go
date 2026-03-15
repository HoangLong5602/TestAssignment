package handler

const (
	ContentTypeJSON       = "application/json"
	HeaderContentType     = "Content-Type"
	ErrMethodNotAllowed   = "method not allowed"
	ErrRoomIDRequired     = "room_id required"
	ErrUserAndRoomRequired = "user_id and room_id required"
	ErrJoinRoomFirst      = "join room first via API"
	ErrAlreadyInOtherRoom = "already in another room"
	ErrRoomNotFound       = "room not found"
	ErrRoomNotAccepting   = "room not accepting joins"
	ErrRoomNotWaiting     = "room not in waiting state"
	ErrFailedToStart      = "failed to start"
	ErrInvalidBody        = "invalid body"
	ErrUserInOtherRoom    = "user_already_in_another_room"
	ErrRoomFull           = "room_full"
	ErrDB                 = "db_error"
)

const (
	QueryUserID = "user_id"
	QueryRoomID = "room_id"
)

const (
	WSMsgQuestion          = "question"
	WSMsgLeaderboardUpdate = "leaderboard_update"
	WSMsgAnswerResult      = "answer_result"
	WSMsgQuizEnd           = "quiz_end"
	WSKeyLeaderboard       = "leaderboard"
	WSKeyResult            = "result"
)
