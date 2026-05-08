package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
	webpush "github.com/SherClockHolmes/webpush-go"
	"golang.org/x/crypto/bcrypt"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024 * 1024 * 10,
	WriteBufferSize: 1024 * 1024 * 10,
}

var db *sql.DB

const jwtSecret = "kittychat-secret-key-2024"
const vapidPublicKey = "BK-2ymakSCkhmduqNtSfl97a3o5MyZk3JgAYkJNzH2cHdPRtoOQnFB7a8WYlQpBYckR9Ork7LaIyJWSDu5VPmNQ"
const vapidPrivateKey = "17q5M8mIMzbJpawS60voCoZEvfhV3S84_BHYZqJ0fT0"
const vapidEmail = "mailto:dertdert2003@gmail.com"

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", "/app/data/messages.db")
	if err != nil {
		log.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE,
		password TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		log.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS push_subscriptions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT,
		endpoint TEXT UNIQUE,
		p256dh TEXT,
		auth TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		log.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS public_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE,
		public_key TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		log.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		room TEXT,
		username TEXT,
		type TEXT,
		content TEXT,
		image TEXT,
		audio TEXT,
		file TEXT,
		filename TEXT,
		status TEXT DEFAULT 'sent',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		log.Fatal(err)
	}
	// Индексы для быстрого поиска
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_messages_room ON messages(room)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_messages_created ON messages(created_at)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_messages_username ON messages(username)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_push_username ON push_subscriptions(username)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_public_keys_username ON public_keys(username)`)
	log.Println("Database initialized")
}

func generateToken(username string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": username,
		"exp":      time.Now().Add(30 * 24 * time.Hour).Unix(),
	})
	return token.SignedString([]byte(jwtSecret))
}

func validateToken(tokenStr string) (string, bool) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return []byte(jwtSecret), nil
	})
	if err != nil || !token.Valid {
		return "", false
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", false
	}
	username, ok := claims["username"].(string)
	return username, ok
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Username == "" || req.Password == "" {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "Введи имя и пароль"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		w.WriteHeader(500)
		return
	}
	_, err = db.Exec(`INSERT INTO users (username, password) VALUES (?, ?)`, req.Username, string(hash))
	if err != nil {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "Имя уже занято"})
		return
	}
	token, _ := generateToken(req.Username)
	json.NewEncoder(w).Encode(map[string]string{"token": token, "username": req.Username})
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	var hash string
	err := db.QueryRow(`SELECT password FROM users WHERE username = ?`, req.Username).Scan(&hash)
	if err != nil {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]string{"error": "Неверное имя или пароль"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]string{"error": "Неверное имя или пароль"})
		return
	}
	token, _ := generateToken(req.Username)
	json.NewEncoder(w).Encode(map[string]string{"token": token, "username": req.Username})
}

func saveMessage(msg Message) int64 {
	result, err := db.Exec(
		`INSERT INTO messages (room, username, type, content, image, audio, file, filename, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'sent')`,
		msg.Room, msg.Username, msg.Type, msg.Content, msg.Image, msg.Audio, msg.File, msg.Filename,
	)
	if err != nil {
		log.Println("Error saving message:", err)
		return 0
	}
	id, _ := result.LastInsertId()
	return id
}

func updateMessageStatus(id int64, status string) {
	db.Exec(`UPDATE messages SET status = ? WHERE id = ?`, status, id)
}

func getHistory(room string) []Message {
	rows, err := db.Query(
    	     `SELECT id, username, type, content, COALESCE(image,''), COALESCE(audio,''), COALESCE(file,''), COALESCE(filename,'') FROM messages WHERE room = ? AND (content != '' OR image IS NOT NULL OR audio IS NOT NULL OR file IS NOT NULL) ORDER BY created_at DESC LIMIT 50`,
    	     room,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var messages []Message
	for rows.Next() {
		var msg Message
		rows.Scan(&msg.ID, &msg.Username, &msg.Type, &msg.Content, &msg.Image, &msg.Audio, &msg.File, &msg.Filename)
		msg.Room = room
		messages = append(messages, msg)
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages
}

type Client struct {
	conn     *websocket.Conn
	send     chan []byte
	username string
	room     string
}

type Message struct {
	Type     string   `json:"type"`
	ID       int64    `json:"id,omitempty"`
	Username string   `json:"username"`
	Room     string   `json:"room"`
	Content  string   `json:"content"`
	Image    string   `json:"image,omitempty"`
	Audio    string   `json:"audio,omitempty"`
	File     string   `json:"file,omitempty"`
	Filename string   `json:"filename,omitempty"`
	Users    []string `json:"users,omitempty"`
	Status   string   `json:"status,omitempty"`
}

type Hub struct {
	rooms      map[string]map[*Client]bool
	broadcast  chan Message
	register   chan *Client
	unregister chan *Client
	mu         sync.Mutex
}

var hub = &Hub{
	rooms:      make(map[string]map[*Client]bool),
	broadcast:  make(chan Message, 256),
	register:   make(chan *Client),
	unregister: make(chan *Client),
}

func (h *Hub) broadcastUsers(room string) {
	var users []string
	for client := range h.rooms[room] {
		users = append(users, client.username)
	}
	msg := Message{Type: "users", Room: room, Users: users}
	data, _ := json.Marshal(msg)
	for client := range h.rooms[room] {
		select {
		case client.send <- data:
		default:
			close(client.send)
			delete(h.rooms[room], client)
		}
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if h.rooms[client.room] == nil {
				h.rooms[client.room] = make(map[*Client]bool)
			}
			h.rooms[client.room][client] = true
			h.broadcastUsers(client.room)
			h.mu.Unlock()
			log.Printf("%s joined room %s", client.username, client.room)
			history := getHistory(client.room)
			for _, msg := range history {
				msg.Type = "history"
				data, _ := json.Marshal(msg)
				client.send <- data
			}

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.rooms[client.room][client]; ok {
				delete(h.rooms[client.room], client)
				close(client.send)
				h.broadcastUsers(client.room)
			}
			h.mu.Unlock()
			log.Printf("%s left room %s", client.username, client.room)
		case msg := <-h.broadcast:
                        if msg.Type == "read" {
                                h.mu.Lock()
                                statusMsg := Message{Type: "status", Room: msg.Room, Status: "read", Username: msg.Username}
                                data, _ := json.Marshal(statusMsg)
                                for client := range h.rooms[msg.Room] {
                                        select {
                                        case client.send <- data:
                                        default:
                                        }
                                }
                                h.mu.Unlock()
                                continue
                        }
			if msg.Type == "call-offer" || msg.Type == "call-answer" || msg.Type == "call-ice" {
			    log.Printf("CALL SIGNAL: type=%s from=%s room=%s", msg.Type, msg.Username, msg.Room)
			    h.mu.Lock()
                                data, _ := json.Marshal(msg)
                                for client := range h.rooms[msg.Room] {
                                        if client.username == msg.Username {
                                                continue
                                        }
                                        select {
                                        case client.send <- data:
                                        default:
                                        }
                                }
                                h.mu.Unlock()
                                continue
                        }
                        if msg.Type == "call-end" {
                                duration := msg.Content
                                callMsg := Message{
                                        Type:     "message",
                                        Username: msg.Username,
                                        Room:     msg.Room,
                                        Content:  "📞 Звонок завершён" + func() string {
                                                if duration != "" { return " · " + duration }
                                                return ""
                                        }(),
                                }
                                msgID := saveMessage(callMsg)
                                callMsg.ID = msgID
                                h.mu.Lock()
                                data, _ := json.Marshal(callMsg)
                                for client := range h.rooms[msg.Room] {
                                        select {
                                        case client.send <- data:
                                        default:
                                        }
                                }
                                h.mu.Unlock()
                                continue
                        }
                        msgID := saveMessage(msg)
                        msg.ID = msgID
                        msg.Status = "sent"
                        h.mu.Lock()
                        clients := h.rooms[msg.Room]
                        delivered := len(clients) > 1
                        if delivered {
                                msg.Status = "delivered"
                                updateMessageStatus(msgID, "delivered")
                        }
                        data, _ := json.Marshal(msg)
                        onlineUsers := make(map[string]bool)
                        for client := range clients {
                                onlineUsers[client.username] = true
                                select {
                                case client.send <- data:
                                default:
                                        close(client.send)
                                        delete(h.rooms[msg.Room], client)
                                }
                        }
                        h.mu.Unlock()
                        if len(msg.Room) > 3 && msg.Room[:3] == "dm_" {
                                rows, _ := db.Query(`SELECT DISTINCT username FROM push_subscriptions`)
                                if rows != nil {
                                        defer rows.Close()
                                        for rows.Next() {
                                                var u string
                                                rows.Scan(&u)
                                                if u != msg.Username && !onlineUsers[u] {
                                                        go sendPushNotification(u, msg.Username, msg.Content)
                                                }
                                        }
                                }
                        }
		}
	}
}

func (c *Client) writePump() {
        defer c.conn.Close()
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        for {
                select {
                case msg, ok := <-c.send:
                        if !ok {
                                c.conn.WriteMessage(websocket.CloseMessage, []byte{})
                                return
                        }
                        c.conn.WriteMessage(websocket.TextMessage, msg)
                case <-ticker.C:
                        if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                                return
                        }
                }
        }
}
func (c *Client) readPump() {
	defer func() {
		hub.unregister <- c
		c.conn.Close()
	}()
	for {
		c.conn.SetReadLimit(10 * 1024 * 1024)
		c.conn.SetPongHandler(func(string) error {
		        c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		        return nil
		})
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		msg.Username = c.username
		msg.Room = c.room
		hub.broadcast <- msg
	}
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	room := r.URL.Query().Get("room")
	username, valid := validateToken(token)
	if !valid || room == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	client := &Client{
		conn:     conn,
		send:     make(chan []byte, 256),
		username: username,
		room:     room,
	}
	hub.register <- client
	go client.writePump()
	go client.readPump()
}


func subscribeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	token := r.URL.Query().Get("token")
	username, valid := validateToken(token)
	if !valid {
		w.WriteHeader(401)
		return
	}
	var req struct {
		Endpoint string `json:"endpoint"`
		P256dh   string `json:"p256dh"`
		Auth     string `json:"auth"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Endpoint == "" {
		w.WriteHeader(400)
		return
	}
	_, err := db.Exec(`INSERT INTO push_subscriptions (username, endpoint, p256dh, auth) VALUES (?, ?, ?, ?)
		ON CONFLICT(endpoint) DO UPDATE SET username=excluded.username, p256dh=excluded.p256dh, auth=excluded.auth`,
		username, req.Endpoint, req.P256dh, req.Auth)
	if err != nil {
		log.Println("Error saving subscription:", err)
		w.WriteHeader(500)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func vapidKeyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"public_key": vapidPublicKey})
}

func sendPushNotification(username string, title string, body string) {
	log.Printf("Sending push to %s: %s", username, body)
	rows, err := db.Query(`SELECT endpoint, p256dh, auth FROM push_subscriptions WHERE username = ?`, username)
	if err != nil {
		return
	}
	defer rows.Close()
	payload := `{"title":"` + title + `","body":"` + body + `"}`
	for rows.Next() {
		var endpoint, p256dh, auth string
		rows.Scan(&endpoint, &p256dh, &auth)
		sub := &webpush.Subscription{
			Endpoint: endpoint,
			Keys: webpush.Keys{
				P256dh: p256dh,
				Auth:   auth,
			},
		}
		go func() {
		    resp, err := webpush.SendNotification([]byte(payload), sub, &webpush.Options{
		        VAPIDPublicKey:  vapidPublicKey,
		        VAPIDPrivateKey: vapidPrivateKey,
		        TTL:             30,
		        Subscriber:      vapidEmail,
		    })
		    if err != nil {
		        log.Printf("Push error: %v", err)
		    } else {
		        log.Printf("Push response: %d", resp.StatusCode)
		        resp.Body.Close()
		    }
		}()
	}
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	token := r.URL.Query().Get("token")
	_, valid := validateToken(token)
	if !valid {
		w.WriteHeader(401)
		return
	}
	query := r.URL.Query().Get("q")
	room := r.URL.Query().Get("room")
	if query == "" {
		w.WriteHeader(400)
		return
	}
	var rows *sql.Rows
	var err error
	if room != "" {
		rows, err = db.Query(
			`SELECT username, type, content, COALESCE(image,''), COALESCE(audio,''), COALESCE(file,''), COALESCE(filename,''), room FROM messages WHERE room = ? AND content LIKE ? ORDER BY created_at DESC LIMIT 20`,
			room, "%"+query+"%")
	} else {
		rows, err = db.Query(
			`SELECT username, type, content, COALESCE(image,''), COALESCE(audio,''), COALESCE(file,''), COALESCE(filename,''), room FROM messages WHERE content LIKE ? ORDER BY created_at DESC LIMIT 20`,
			"%"+query+"%")
	}
	if err != nil {
		w.WriteHeader(500)
		return
	}
	defer rows.Close()
	var messages []Message
	for rows.Next() {
		var msg Message
		rows.Scan(&msg.Username, &msg.Type, &msg.Content, &msg.Image, &msg.Audio, &msg.File, &msg.Filename, &msg.Room)
		messages = append(messages, msg)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"messages": messages})
}

func deleteMessageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	token := r.URL.Query().Get("token")
	username, valid := validateToken(token)
	if !valid {
		w.WriteHeader(401)
		return
	}
	var req struct {
		ID   int64  `json:"id"`
		Room string `json:"room"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	// Проверяем что сообщение принадлежит пользователю
	var owner string
	err := db.QueryRow(`SELECT username FROM messages WHERE id = ?`, req.ID).Scan(&owner)
	if err != nil || owner != username {
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]string{"error": "Нельзя удалить чужое сообщение"})
		return
	}
	db.Exec(`DELETE FROM messages WHERE id = ?`, req.ID)
	// Уведомляем всех в комнате
	hub.mu.Lock()
	deleteMsg := Message{Type: "deleted", ID: req.ID, Room: req.Room}
	data, _ := json.Marshal(deleteMsg)
	for client := range hub.rooms[req.Room] {
		select {
		case client.send <- data:
		default:
		}
	}
	hub.mu.Unlock()
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func editMessageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	token := r.URL.Query().Get("token")
	username, valid := validateToken(token)
	if !valid {
		w.WriteHeader(401)
		return
	}
	var req struct {
		ID      int64  `json:"id"`
		Room    string `json:"room"`
		Content string `json:"content"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	var owner string
	err := db.QueryRow(`SELECT username FROM messages WHERE id = ?`, req.ID).Scan(&owner)
	if err != nil || owner != username {
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]string{"error": "Нельзя редактировать чужое сообщение"})
		return
	}
	db.Exec(`UPDATE messages SET content = ? WHERE id = ?`, req.Content, req.ID)
	// Уведомляем всех в комнате
	hub.mu.Lock()
	editMsg := Message{Type: "edited", ID: req.ID, Room: req.Room, Content: req.Content}
	data, _ := json.Marshal(editMsg)
	for client := range hub.rooms[req.Room] {
		select {
		case client.send <- data:
		default:
		}
	}
	hub.mu.Unlock()
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func saveKeyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	token := r.URL.Query().Get("token")
	username, valid := validateToken(token)
	if !valid {
		w.WriteHeader(401)
		return
	}
	var req struct {
		PublicKey string `json:"public_key"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.PublicKey == "" {
		w.WriteHeader(400)
		return
	}
	_, err := db.Exec(`INSERT INTO public_keys (username, public_key) VALUES (?, ?)
		ON CONFLICT(username) DO UPDATE SET public_key=excluded.public_key, updated_at=CURRENT_TIMESTAMP`,
		username, req.PublicKey)
	if err != nil {
		log.Println("Error saving key:", err)
		w.WriteHeader(500)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func getKeyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	token := r.URL.Query().Get("token")
	_, valid := validateToken(token)
	if !valid {
		w.WriteHeader(401)
		return
	}
	target := r.URL.Query().Get("username")
	if target == "" {
		w.WriteHeader(400)
		return
	}
	var publicKey string
	err := db.QueryRow(`SELECT public_key FROM public_keys WHERE username = ?`, target).Scan(&publicKey)
	if err != nil {
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "Ключ не найден"})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"public_key": publicKey, "username": target})
}

func dmHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	token := r.URL.Query().Get("token")
	target := r.URL.Query().Get("target")
	username, valid := validateToken(token)
	if !valid || target == "" {
		w.WriteHeader(401)
		return
	}
	a, b := username, target
	if a > b {
		a, b = b, a
	}
	room := "dm_" + a + "_" + b
	json.NewEncoder(w).Encode(map[string]string{"room": room, "target": target})
}

func usersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	token := r.URL.Query().Get("token")
	_, valid := validateToken(token)
	if !valid {
		w.WriteHeader(401)
		return
	}
	rows, err := db.Query("SELECT username FROM users ORDER BY username")
	if err != nil {
		w.WriteHeader(500)
		return
	}
	defer rows.Close()
	var users []string
	for rows.Next() {
		var u string
		rows.Scan(&u)
		users = append(users, u)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"users": users})
}

func main() {
	initDB()
	go hub.run()
	http.HandleFunc("/register", registerHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/vapid-key", vapidKeyHandler)
	http.HandleFunc("/subscribe", subscribeHandler)
	http.HandleFunc("/search", searchHandler)
	http.HandleFunc("/message/delete", deleteMessageHandler)
	http.HandleFunc("/message/edit", editMessageHandler)
	http.HandleFunc("/keys", saveKeyHandler)
	http.HandleFunc("/keys/get", getKeyHandler)
	http.HandleFunc("/dm", dmHandler)
	http.HandleFunc("/users", usersHandler)
	http.HandleFunc("/ws", wsHandler)
	http.HandleFunc("/icon.png", func(w http.ResponseWriter, r *http.Request) {
	    w.Header().Set("Content-Type", "image/png")
	    http.ServeFile(w, r, "icon.png")
	})
	http.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
	    w.Header().Set("Content-Type", "application/manifest+json")
	    http.ServeFile(w, r, "manifest.json")
	})
	http.HandleFunc("/sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, r, "sw.js")
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "client.html")
	})
	log.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
