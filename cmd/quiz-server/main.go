package main

import (
	"context"
	"embed"
	"io/fs"
	"learning/internal/config"
	"learning/internal/handler"
	"learning/internal/hub"
	"learning/internal/store"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

//go:embed testclient
var testclientFS embed.FS

func main() {
	cfg := config.Load()
	s := store.New(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	defer s.Close()

	if os.Getenv("CLEAR_USER_ROOM_ON_STARTUP") == "1" || os.Getenv("CLEAR_USER_ROOM_ON_STARTUP") == "true" {
		if err := s.ClearAllUserRooms(context.Background()); err != nil {
			log.Printf("ClearAllUserRooms: %v", err)
		} else {
			log.Println("cleared all user-room bindings (restart clean state)")
		}
	}

	h := hub.NewHub()
	go h.Run()

	hand := handler.New(cfg, s, h)
	handler.WireInboundHandler(hand)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handler.RunLeaderboardSubscriber(ctx, s, h)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /room", hand.CreateRoom)
	mux.HandleFunc("POST /room/join", hand.JoinRoom)
	mux.HandleFunc("POST /room/start", hand.StartQuiz)
	mux.HandleFunc("POST /room/next", hand.NextQuestion)
	mux.HandleFunc("POST /room/end", hand.EndQuiz)
	mux.HandleFunc("GET /ws", hand.ServeWS)
	sub, _ := fs.Sub(testclientFS, "testclient")
	mux.Handle("GET /test/", http.StripPrefix("/test", http.FileServer(http.FS(sub))))
	mux.HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/test/", 302) })

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: mux}
	go func() {
		log.Println("server listening on", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")
	cancel()
	_ = srv.Shutdown(context.Background())
}
