package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// User and GameRoom structures
type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
	EXP      int    `json:"exp"`
	Level    int    `json:"level"`
}

type UsersData struct {
	Users []User `json:"users"`
}

type GameRoom struct {
	ID      string
	Host    string
	Guest   string
	Started bool
}

var (
	usersLock sync.Mutex
	usersFile = "data/users.json"
	gameRooms = make(map[string]*GameRoom)
	roomsLock sync.Mutex
)

// Tower and Troop specs (Simple TCR, hardcoded for now)
// REMOVE any hardcoded troopTemplates or towerTemplates here

// Declare global variables for loaded specs
var (
	towerSpecs []TowerSpec
	troopSpecs []TroopSpec
)

// Tower and Troop specs (Simple TCR, hardcoded for now)
type Tower struct {
	Name string
	HP   int
	ATK  int
	DEF  int
}

type Troop struct {
	Name  string
	HP    int
	ATK   int
	DEF   int
	Owner string // Username
	Alive bool
}

type PlayerState struct {
	Username string
	Towers   map[string]*Tower
	Troops   []*Troop
	Turn     bool
}

type GameState struct {
	RoomID   string
	Players  map[string]*PlayerState // username -> state
	TurnUser string
	Winner   string
	Over     bool
}

var (
	games     = make(map[string]*GameState)
	gamesLock sync.Mutex
)

// Enhanced TCR: Add CRIT, MANA, EXP, Leveling, Timer, JSON specs
type TowerSpec struct {
	Name string  `json:"name"`
	HP   int     `json:"hp"`
	ATK  int     `json:"atk"`
	DEF  int     `json:"def"`
	CRIT float64 `json:"crit"`
	EXP  int     `json:"exp"`
}

type TroopSpec struct {
	Name    string `json:"name"`
	HP      int    `json:"hp"`
	ATK     int    `json:"atk"`
	DEF     int    `json:"def"`
	MANA    int    `json:"mana"`
	EXP     int    `json:"exp"`
	Special string `json:"special"`
}

type PlayerProgress struct {
	Username string         `json:"username"`
	EXP      int            `json:"exp"`
	Level    int            `json:"level"`
	TowerLv  map[string]int `json:"tower_lv"`
	TroopLv  map[string]int `json:"troop_lv"`
}

// Enhanced PlayerState for mana, exp, etc.
type EnhancedPlayerState struct {
	Username string
	Towers   map[string]*Tower
	Troops   []*Troop
	Mana     int
	EXP      int
	Level    int
	Progress *PlayerProgress
}

type EnhancedGameState struct {
	RoomID    string
	Players   map[string]*EnhancedPlayerState
	Winner    string
	Over      bool
	StartTime time.Time
	EndTime   time.Time
}

var (
	enhancedGames     = make(map[string]*EnhancedGameState)
	enhancedGamesLock sync.Mutex
	specsFile         = "data/specs.json"
	progressFile      = "data/users.json"
)

func main() {
	fmt.Println("TCR Server starting...")
	ln, err := net.Listen("tcp", ":9000")
	if err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}
	defer ln.Close()
	fmt.Println("Server listening on :9000")
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Accept error:", err)
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	send := func(msg string) { conn.Write([]byte(msg + "\n")) }
	scanner := bufio.NewScanner(conn)
	var currentUser *User
	var currentUsername string
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "|", 3)
		cmd := strings.ToUpper(parts[0])
		switch cmd {
		case "LOGIN":
			if len(parts) < 3 {
				send("ERR|Usage: LOGIN|username|password")
				continue
			}
			user := authenticate(parts[1], parts[2])
			if user != nil {
				currentUser = user
				currentUsername = user.Username
				userConns.Store(user.Username, conn)
				send("ACK|Login successful")
			} else {
				send("ERR|Invalid credentials")
			}
		case "REGISTER":
			if len(parts) < 3 {
				send("ERR|Usage: REGISTER|username|password")
				continue
			}
			if registerUser(parts[1], parts[2]) {
				send("ACK|Registration successful")
			} else {
				send("ERR|Username taken")
			}
		case "CREATE_GAME":
			if currentUser == nil {
				send("ERR|Login first")
				continue
			}
			mode := "SIMPLE"
			if len(parts) > 1 {
				mode = strings.ToUpper(parts[1])
			}
			roomID := createGameRoom(currentUser.Username, mode)
			send("ACK|GAME_CREATED|" + roomID)
		case "LIST_GAMES":
			if currentUser == nil {
				send("ERR|Login first")
				continue
			}
			list := listGameRooms()
			send("GAMES|" + list)
		case "JOIN_GAME":
			if currentUser == nil || len(parts) < 2 {
				send("ERR|Usage: JOIN_GAME|room_id")
				continue
			}
			ok := joinGameRoom(parts[1], currentUser.Username)
			if ok {
				// Khi đủ 2 người, tự động start game và gửi ACK|GAME_STARTED cho cả hai
				room := gameRooms[parts[1]]
				if room != nil && room.Host != "" && room.Guest != "" {
					startGame(parts[1], room.Host, nil)
					notifyGameStartedWithTurn(room.ID)
				}
				send("ACK|JOINED|" + parts[1])
			} else {
				send("ERR|Cannot join game")
			}
		case "START_GAME":
			if currentUser == nil {
				send("ERR|Login first")
				continue
			}
			if len(parts) < 2 {
				send("ERR|Usage: START_GAME|room_id")
				continue
			}
			ok := startGame(parts[1], currentUser.Username, conn)
			if ok {
				send("ACK|GAME_STARTED")
			} else {
				send("ERR|Cannot start game")
			}
		case "DEPLOY":
			if currentUser == nil {
				send("ERR|Login first")
				continue
			}
			if len(parts) < 3 {
				send("ERR|Usage: DEPLOY|troop_name|target_tower")
				continue
			}
			msg := handleDeploy(currentUser.Username, parts[1], parts[2])
			send(msg)
		case "STATE":
			if currentUser == nil {
				send("ERR|Login first")
				continue
			}
			msg := getGameState(currentUser.Username)
			send(msg)
		default:
			send("ERR|Unknown command")
		}
	}
	if currentUsername != "" {
		userConns.Delete(currentUsername)
	}
}

// Global map for user connections
var userConns sync.Map // username -> net.Conn

func registerConn(user *User, conn net.Conn) {
	if user != nil {
		userConns.Store(user.Username, conn)
	}
}

func notifyGameStartedWithTurn(roomID string) {
	gamesLock.Lock()
	game, ok := games[roomID]
	gamesLock.Unlock()
	if !ok {
		return
	}
	for uname := range game.Players {
		if v, ok := userConns.Load(uname); ok {
			if conn, ok2 := v.(net.Conn); ok2 {
				// Gửi thông báo game đã bắt đầu
				conn.Write([]byte("ACK|GAME_STARTED\n"))

				// Thêm delay nhỏ để đảm bảo các thông báo không đến quá gần nhau
				time.Sleep(100 * time.Millisecond)

				// Gửi trạng thái game ban đầu
				stateMsg := "STATE|" + formatGameState(game, uname) + "\n"
				conn.Write([]byte(stateMsg))

				// Thêm delay nhỏ để đảm bảo các thông báo không đến quá gần nhau
				time.Sleep(100 * time.Millisecond)

				// Gửi thông báo lượt chơi
				if uname == game.TurnUser {
					conn.Write([]byte("TURN|Your turn!\n"))
				} else {
					conn.Write([]byte("TURN|Wait for your turn...\n"))
				}
			}
		}
	}
}

func authenticate(username, password string) *User {
	usersLock.Lock()
	defer usersLock.Unlock()
	data, err := ioutil.ReadFile(usersFile)
	if err != nil {
		return nil
	}
	var users UsersData
	if err := json.Unmarshal(data, &users); err != nil {
		return nil
	}
	for _, u := range users.Users {
		if u.Username == username && u.Password == password {
			return &u
		}
	}
	return nil
}

func registerUser(username, password string) bool {
	usersLock.Lock()
	defer usersLock.Unlock()
	data, err := ioutil.ReadFile(usersFile)
	if err != nil {
		return false
	}
	var users UsersData
	if err := json.Unmarshal(data, &users); err != nil {
		return false
	}
	for _, u := range users.Users {
		if u.Username == username {
			return false
		}
	}
	newUser := User{Username: username, Password: password, EXP: 0, Level: 1}
	users.Users = append(users.Users, newUser)
	out, _ := json.MarshalIndent(users, "", "  ")
	_ = ioutil.WriteFile(usersFile, out, 0644)
	return true
}

// Add support for CREATE_GAME and JOIN_GAME with mode
func createGameRoom(host, mode string) string {
	roomsLock.Lock()
	defer roomsLock.Unlock()
	id := fmt.Sprintf("room%d", len(gameRooms)+1)
	gameRooms[id] = &GameRoom{ID: id, Host: host, Started: false}
	// Store mode in room ID for now (e.g., room1:ENHANCED)
	if mode == "ENHANCED" {
		id = id + ":ENHANCED"
	}
	return id
}

func listGameRooms() string {
	roomsLock.Lock()
	defer roomsLock.Unlock()
	var ids []string
	for id, room := range gameRooms {
		if !room.Started && room.Guest == "" {
			ids = append(ids, id+":"+room.Host)
		}
	}
	return strings.Join(ids, ",")
}

func joinGameRoom(id, guest string) bool {
	roomsLock.Lock()
	defer roomsLock.Unlock()
	room, ok := gameRooms[id]
	if !ok || room.Started || room.Guest != "" {
		return false
	}
	room.Guest = guest
	room.Started = true
	return true
}

// Game logic functions
func startGame(roomID, username string, conn net.Conn) bool {
	gamesLock.Lock()
	defer gamesLock.Unlock()
	room, ok := gameRooms[roomID]
	if !ok || !room.Started {
		return false
	}
	// Only start if both players are present
	if room.Host == "" || room.Guest == "" {
		return false
	}
	if _, exists := games[roomID]; exists {
		return false
	}
	// Initialize game state
	players := map[string]*PlayerState{}
	// Build a map of available troop specs by name
	troopSpecMap := map[string]TroopSpec{}
	for _, t := range troopSpecs {
		troopSpecMap[t.Name] = t
	}
	// Get all available troop names
	availableTroopNames := make([]string, 0, len(troopSpecMap))
	for name := range troopSpecMap {
		availableTroopNames = append(availableTroopNames, name)
	}
	rand.Seed(time.Now().UnixNano())
	for _, uname := range []string{room.Host, room.Guest} {
		// Randomly select 3 unique troops for each player
		troopNames := make([]string, len(availableTroopNames))
		copy(troopNames, availableTroopNames)
		rand.Shuffle(len(troopNames), func(i, j int) { troopNames[i], troopNames[j] = troopNames[j], troopNames[i] })
		selected := troopNames[:3]
		troops := []*Troop{}
		for _, tn := range selected {
			spec := troopSpecMap[tn]
			troops = append(troops, &Troop{
				Name:  spec.Name,
				HP:    spec.HP,
				ATK:   spec.ATK,
				DEF:   spec.DEF,
				Owner: uname,
				Alive: true,
			})
		}
		// Assign towers from towerSpecs
		towers := map[string]*Tower{}
		for _, ts := range towerSpecs {
			towers[ts.Name] = &Tower{
				Name: ts.Name,
				HP:   ts.HP,
				ATK:  ts.ATK,
				DEF:  ts.DEF,
			}
		}
		players[uname] = &PlayerState{
			Username: uname,
			Towers:   towers,
			Troops:   troops,
			Turn:     false,
		}
	}
	// Randomly pick who starts
	turnIdx := rand.Intn(2)
	turnUser := room.Host
	if turnIdx == 1 {
		turnUser = room.Guest
	}
	players[turnUser].Turn = true
	games[roomID] = &GameState{
		RoomID:   roomID,
		Players:  players,
		TurnUser: turnUser,
		Winner:   "",
		Over:     false,
	}
	return true
}

func handleDeploy(username, troopName, towerName string) string {
	gamesLock.Lock()
	defer gamesLock.Unlock()
	var game *GameState
	for _, g := range games {
		if _, ok := g.Players[username]; ok {
			game = g
			break
		}
	}
	if game == nil || game.Over {
		return "ERR|No active game"
	}
	if game.TurnUser != username {
		return "ERR|Not your turn"
	}
	player := game.Players[username]
	var troop *Troop
	for _, t := range player.Troops {
		if t.Name == troopName && t.Alive {
			troop = t
			break
		}
	}
	if troop == nil {
		return "ERR|Invalid or dead troop"
	}
	// Find target tower
	var enemyName string
	for uname := range game.Players {
		if uname != username {
			enemyName = uname
			break
		}
	}
	enemy := game.Players[enemyName]
	tower, ok := enemy.Towers[towerName]
	if !ok || tower.HP <= 0 {
		return "ERR|Invalid or destroyed tower"
	}
	// Simple attack logic
	damage := troop.ATK - tower.DEF
	if damage < 0 {
		damage = 0
	}
	tower.HP -= damage
	if tower.HP < 0 {
		tower.HP = 0
	}
	// Mark troop as used (dead for this turn-based version)
	troop.Alive = false
	// Check for win
	if enemy.Towers["King"].HP <= 0 {
		game.Over = true
		game.Winner = username
		sendToUser(username, "GAME_END|You win! King destroyed.")
		sendToUser(enemyName, "GAME_END|You lose! King destroyed.")
		return "STATE|" + formatGameState(game, username)
	}
	// Switch turn
	game.TurnUser = enemyName
	for uname, p := range game.Players {
		if uname == game.TurnUser {
			p.Turn = true
		} else {
			p.Turn = false
		}
	}

	// 1. First send attack result
	attackResult := fmt.Sprintf("ATTACK_RESULT|%s|%s|%d|%d", troopName, towerName, damage, tower.HP)
	sendToUser(username, attackResult)
	sendToUser(enemyName, attackResult)

	// 2. Then send updated state with a small delay to both players
	time.Sleep(100 * time.Millisecond)
	stateMsg := "STATE|" + formatGameState(game, username)
	sendToUser(username, stateMsg)
	sendToUser(enemyName, "STATE|"+formatGameState(game, enemyName))

	// 3. Finally, send turn notifications with a delay
	time.Sleep(100 * time.Millisecond)
	sendToUser(game.TurnUser, "TURN|Your turn!")
	sendToUser(username, "TURN|Wait for your turn...")

	return "ACK|Deploy successful"
}

// Helper to send message to a user if connected
func sendToUser(username, msg string) {
	if v, ok := userConns.Load(username); ok {
		if conn, ok2 := v.(net.Conn); ok2 {
			conn.Write([]byte(msg + "\n"))
		}
	}
}

func getGameState(username string) string {
	gamesLock.Lock()
	defer gamesLock.Unlock()
	for _, g := range games {
		if g.Players[username] != nil {
			return "STATE|" + formatGameState(g, username)
		}
	}
	return "ERR|Not in game"
}

func formatGameState(g *GameState, username string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Room: %s\n", g.RoomID))
	for uname, ps := range g.Players {
		sb.WriteString(fmt.Sprintf("Player: %s %s\n", uname, ternary(uname == g.TurnUser, "(TURN)", "")))
		sb.WriteString("  Towers:\n")
		for _, t := range []string{"Guard1", "Guard2", "King"} {
			tower := ps.Towers[t]
			sb.WriteString(fmt.Sprintf("    %s: HP=%d ATK=%d DEF=%d\n", tower.Name, tower.HP, tower.ATK, tower.DEF))
		}
		sb.WriteString("  Troops:\n")
		for _, tr := range ps.Troops {
			sb.WriteString(fmt.Sprintf("    %s (Alive: %v)\n", tr.Name, tr.Alive))
		}
	}
	sb.WriteString(fmt.Sprintf("Current turn: %s\n", g.TurnUser))
	if g.Winner != "" {
		sb.WriteString(fmt.Sprintf("Winner: %s\n", g.Winner))
	}
	return sb.String()
}

func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func loadSpecs() {
	data, err := ioutil.ReadFile(specsFile)
	if err != nil {
		fmt.Println("Error loading specs.json:", err)
		os.Exit(1)
	}
	var specs struct {
		Towers []TowerSpec `json:"towers"`
		Troops []TroopSpec `json:"troops"`
	}
	if err := json.Unmarshal(data, &specs); err != nil {
		fmt.Println("Error parsing specs.json:", err)
		os.Exit(1)
	}
	towerSpecs = specs.Towers
	troopSpecs = specs.Troops
}

func init() {
	loadSpecs()
}

// Load/save player progress (exp, level, etc.)
func loadProgress(username string) *PlayerProgress {
	usersLock.Lock()
	defer usersLock.Unlock()
	data, err := ioutil.ReadFile(progressFile)
	if err != nil {
		return &PlayerProgress{Username: username, Level: 1, TowerLv: map[string]int{}, TroopLv: map[string]int{}}
	}
	var users UsersData
	_ = json.Unmarshal(data, &users)
	for _, u := range users.Users {
		if u.Username == username {
			return &PlayerProgress{Username: u.Username, EXP: u.EXP, Level: u.Level, TowerLv: map[string]int{}, TroopLv: map[string]int{}}
		}
	}
	return &PlayerProgress{Username: username, Level: 1, TowerLv: map[string]int{}, TroopLv: map[string]int{}}
}

func saveProgress(progress *PlayerProgress) {
	usersLock.Lock()
	defer usersLock.Unlock()
	data, err := ioutil.ReadFile(progressFile)
	if err != nil {
		return
	}
	var users UsersData
	_ = json.Unmarshal(data, &users)
	for i, u := range users.Users {
		if u.Username == progress.Username {
			users.Users[i].EXP = progress.EXP
			users.Users[i].Level = progress.Level
		}
	}
	out, _ := json.MarshalIndent(users, "", "  ")
	_ = ioutil.WriteFile(progressFile, out, 0644)
}

// Enhanced game start: continuous, mana, exp, timer
func startEnhancedGame(roomID string) bool {
	enhancedGamesLock.Lock()
	defer enhancedGamesLock.Unlock()
	room, ok := gameRooms[roomID]
	if !ok || !room.Started {
		return false
	}
	if room.Host == "" || room.Guest == "" {
		return false
	}
	if _, exists := enhancedGames[roomID]; exists {
		return false
	}
	players := map[string]*EnhancedPlayerState{}
	for _, uname := range []string{room.Host, room.Guest} {
		progress := loadProgress(uname)
		towers := map[string]*Tower{}
		for _, v := range towerSpecs {
			// Scale stats by level
			lv := progress.TowerLv[v.Name]
			if lv == 0 {
				lv = 1
			}
			mult := 1.0 + 0.1*float64(lv-1)
			towers[v.Name] = &Tower{
				Name: v.Name,
				HP:   int(float64(v.HP) * mult),
				ATK:  int(float64(v.ATK) * mult),
				DEF:  int(float64(v.DEF) * mult),
			}
		}
		troops := []*Troop{}
		for _, t := range troopSpecs {
			lv := progress.TroopLv[t.Name]
			if lv == 0 {
				lv = 1
			}
			mult := 1.0 + 0.1*float64(lv-1)
			troops = append(troops, &Troop{
				Name:  t.Name,
				HP:    int(float64(t.HP) * mult),
				ATK:   int(float64(t.ATK) * mult),
				DEF:   int(float64(t.DEF) * mult),
				Owner: uname,
				Alive: true,
			})
		}
		players[uname] = &EnhancedPlayerState{
			Username: uname,
			Towers:   towers,
			Troops:   troops,
			Mana:     5,
			EXP:      progress.EXP,
			Level:    progress.Level,
			Progress: progress,
		}
	}
	gs := &EnhancedGameState{
		RoomID:    roomID,
		Players:   players,
		Winner:    "",
		Over:      false,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(3 * time.Minute),
	}
	enhancedGames[roomID] = gs
	go enhancedGameLoop(roomID)
	return true
}

// Enhanced game loop: mana regen, timer, end conditions
func enhancedGameLoop(roomID string) {
	for {
		time.Sleep(1 * time.Second)
		enhancedGamesLock.Lock()
		gs, ok := enhancedGames[roomID]
		if !ok || gs.Over {
			enhancedGamesLock.Unlock()
			return
		}
		for _, ps := range gs.Players {
			if ps.Mana < 10 {
				ps.Mana++
			}
		}
		// Award EXP and handle leveling at game end in enhancedGameLoop
		if time.Now().After(gs.EndTime) || gs.Over {
			gs.Over = true
			// Determine winner by towers destroyed
			var maxTowers int
			var winner string
			var draw bool
			counts := map[string]int{}
			for uname, ps := range gs.Players {
				count := 0
				for _, t := range ps.Towers {
					if t.HP <= 0 {
						count++
					}
				}
				counts[uname] = count
				if count > maxTowers {
					maxTowers = count
					winner = uname
				}
			}
			// Check for draw
			draw = false
			if len(counts) == 2 {
				vals := []int{}
				for _, v := range counts {
					vals = append(vals, v)
				}
				if vals[0] == vals[1] {
					draw = true
				}
			}
			if draw {
				gs.Winner = "DRAW"
			} else {
				gs.Winner = winner
			}
			// Award EXP
			for uname, ps := range gs.Players {
				if gs.Winner == "DRAW" {
					ps.EXP += 10
				} else if gs.Winner == uname {
					ps.EXP += 30
				}
				// Level up if enough EXP
				req := 100 + int(0.1*float64(ps.Level-1)*100)
				for ps.EXP >= req {
					ps.EXP -= req
					ps.Level++
					req = 100 + int(0.1*float64(ps.Level-1)*100)
				}
				ps.Progress.EXP = ps.EXP
				ps.Progress.Level = ps.Level
				saveProgress(ps.Progress)
			}
		}
		enhancedGamesLock.Unlock()
	}
}

// Enhanced deploy: check mana, crit, continuous
func handleEnhancedDeploy(username, troopName, targetTower string) string {
	enhancedGamesLock.Lock()
	defer enhancedGamesLock.Unlock()
	var game *EnhancedGameState
	for _, g := range enhancedGames {
		if g.Players[username] != nil && !g.Over {
			game = g
			break
		}
	}
	if game == nil {
		return "ERR|Not in game"
	}
	ps := game.Players[username]
	var troop *Troop
	for _, t := range ps.Troops {
		if t.Name == troopName && t.Alive {
			troop = t
			break
		}
	}
	if troop == nil {
		return "ERR|No such troop or dead"
	}
	// Check mana
	var tspec TroopSpec
	for _, t := range troopSpecs {
		if t.Name == troopName {
			tspec = t
			break
		}
	}
	if ps.Mana < tspec.MANA {
		return "ERR|Not enough mana"
	}
	ps.Mana -= tspec.MANA
	// Find opponent
	var opp *EnhancedPlayerState
	for uname, p := range game.Players {
		if uname != username {
			opp = p
			break
		}
	}
	if opp == nil {
		return "ERR|No opponent"
	}
	// Check tower attack order
	if targetTower == "Guard2" || targetTower == "King" {
		if opp.Towers["Guard1"].HP > 0 {
			return "ERR|Must destroy Guard1 first"
		}
	}
	tower := opp.Towers[targetTower]
	if tower == nil || tower.HP <= 0 {
		return "ERR|Invalid or destroyed tower"
	}
	// CRIT logic
	critChance := 0.0
	for _, t := range towerSpecs {
		if t.Name == targetTower {
			critChance = t.CRIT
			break
		}
	}
	crit := rand.Float64() < critChance
	atk := troop.ATK
	if crit {
		atk = int(float64(atk) * 1.2)
	}
	dmg := atk - tower.DEF
	if dmg < 0 {
		dmg = 0
	}
	tower.HP -= dmg
	msg := fmt.Sprintf("ATTACK_RESULT|%s|%s|%d|%d|CRIT:%v", troop.Name, tower.Name, dmg, tower.HP, crit)
	if tower.HP <= 0 {
		msg += "|DESTROYED"
		if tower.Name == "King" {
			game.Over = true
			game.Winner = username
			msg += ";GAME_END|" + username + "|King destroyed"
		}
	}
	// Add Queen's heal special in handleEnhancedDeploy
	if troop.Name == "Queen" && tspec.Special == "heal" {
		// Heal the friendly tower with lowest HP by 300
		minHP := 99999
		var healTower *Tower
		for _, t := range ps.Towers {
			if t.HP > 0 && t.HP < minHP {
				minHP = t.HP
				healTower = t
			}
		}
		if healTower != nil {
			healTower.HP += 300
			msg := fmt.Sprintf("QUEEN_HEAL|%s|%d", healTower.Name, healTower.HP)
			return msg
		}
	}
	return msg
}

// Enhanced state
func getEnhancedGameState(username string) string {
	enhancedGamesLock.Lock()
	defer enhancedGamesLock.Unlock()
	for _, g := range enhancedGames {
		if g.Players[username] != nil {
			state, _ := json.Marshal(g)
			return "STATE|" + string(state)
		}
	}
	return "ERR|Not in game"
}
