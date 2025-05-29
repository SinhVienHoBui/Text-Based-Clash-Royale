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
	Mode    string // SIMPLE or ENHANCED
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
	// HP = 0 nghĩa là quân đã chết hoặc đã sử dụng
}

type PlayerState struct {
	Username string
	Towers   map[string]*Tower
	Troops   []*Troop
	Turn     bool
}

type GameState struct {
	RoomID         string
	Players        map[string]*PlayerState // username -> state
	TurnUser       string
	Winner         string
	Over           bool
	AttackPatterns map[string]string // tracks which guard tower each player is attacking first
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
	RoomID         string
	Players        map[string]*EnhancedPlayerState
	Winner         string
	Over           bool
	StartTime      time.Time
	EndTime        time.Time
	AttackPatterns map[string]string // tracks which guard tower each player is attacking first
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
			// Nếu là ENHANCED thì tự động chờ guest vào rồi start game
			if mode == "ENHANCED" {
				go func() {
					for {
						time.Sleep(200 * time.Millisecond)
						roomsLock.Lock()
						room, ok := gameRooms[roomID]
						roomsLock.Unlock()
						if ok && room.Guest != "" && room.Started {
							startEnhancedGame(roomID)
							for _, uname := range []string{room.Host, room.Guest} {
								if v, ok := userConns.Load(uname); ok {
									if conn, ok2 := v.(net.Conn); ok2 {
										conn.Write([]byte("ACK|GAME_STARTED\n"))
										stateMsg := getEnhancedGameState(uname)
										conn.Write([]byte(stateMsg + "\n"))
									}
								}
							}
							break
						}
					}
				}()
			}
			continue
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
					if room.Mode == "ENHANCED" {
						startEnhancedGame(parts[1])
						// Gửi thông báo game đã bắt đầu cho cả hai
						for _, uname := range []string{room.Host, room.Guest} {
							if v, ok := userConns.Load(uname); ok {
								if conn, ok2 := v.(net.Conn); ok2 {
									conn.Write([]byte("ACK|GAME_STARTED\n"))
									// Gửi trạng thái game ban đầu (dạng JSON)
									stateMsg := getEnhancedGameState(uname)
									conn.Write([]byte(stateMsg + "\n"))
								}
							}
						}
					} else {
						startGame(parts[1], room.Host, nil)
						notifyGameStartedWithTurn(room.ID)
					}
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
			// Determine which mode the user is in
			mode := "SIMPLE"
			roomsLock.Lock()
			for _, room := range gameRooms {
				if (room.Host == currentUser.Username || room.Guest == currentUser.Username) && room.Started {
					mode = room.Mode
					break
				}
			}
			roomsLock.Unlock()
			if mode == "ENHANCED" {
				send(handleEnhancedDeploy(currentUser.Username, parts[1], parts[2]))
			} else {
				send(handleDeploy(currentUser.Username, parts[1], parts[2]))
			}
		case "STATE":
			if currentUser == nil {
				send("ERR|Login first")
				continue
			}
			// Determine which mode the user is in
			mode := "SIMPLE"
			roomsLock.Lock()
			for _, room := range gameRooms {
				if (room.Host == currentUser.Username || room.Guest == currentUser.Username) && room.Started {
					mode = room.Mode
					break
				}
			}
			roomsLock.Unlock()
			if mode == "ENHANCED" {
				send(getEnhancedGameState(currentUser.Username))
			} else {
				send(getGameState(currentUser.Username))
			}
		case "EXIT_GAME":
			if currentUser == nil {
				send("ERR|Login first")
				continue
			}
			handlePlayerExit(currentUser.Username)
			// Send GAME_END to the player who exited as well, to ensure they return to menu
			send("GAME_END|You have exited the game")
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
	if mode == "ENHANCED" {
		gameRooms[id] = &GameRoom{ID: id, Host: host, Started: false, Mode: "ENHANCED"}
	} else {
		gameRooms[id] = &GameRoom{ID: id, Host: host, Started: false, Mode: "SIMPLE"}
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
	roomsLock.Lock()
	room, ok := gameRooms[roomID]
	roomsLock.Unlock()
	if !ok || !room.Started {
		return false
	}
	if room.Mode == "ENHANCED" {
		return startEnhancedGame(roomID)
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
	} // Randomly pick who starts
	turnIdx := rand.Intn(2)
	turnUser := room.Host
	if turnIdx == 1 {
		turnUser = room.Guest
	}

	players[turnUser].Turn = true

	games[roomID] = &GameState{
		RoomID:         roomID,
		Players:        players,
		TurnUser:       turnUser,
		Winner:         "",
		Over:           false,
		AttackPatterns: make(map[string]string), // initialize attack pattern tracking
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
		// Special case for Queen who can be used even with HP=0
		if t.Name == troopName && (t.HP > 0 || t.Name == "Queen") {
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

	// Check tower attack restrictions
	if towerName == "King" {
		// Check if at least one guard tower is destroyed before allowing King tower attack
		if enemy.Towers["Guard1"].HP > 0 && enemy.Towers["Guard2"].HP > 0 {
			return "ERR|Must destroy either Guard1 or Guard2 tower before attacking King"
		}
	} else if towerName == "Guard1" || towerName == "Guard2" {
		// Check if this is the first guard tower attack by this player
		firstGuardTower, hasPattern := game.AttackPatterns[username]

		if !hasPattern {
			// First time attacking a guard tower - record the choice
			game.AttackPatterns[username] = towerName
		} else if firstGuardTower != towerName {
			// Player is trying to attack the other guard tower
			if enemy.Towers[firstGuardTower].HP > 0 {
				// The first guard tower is not yet destroyed, prevent switch
				return fmt.Sprintf("ERR|You must destroy %s Tower first before attacking %s Tower", firstGuardTower, towerName)
			}
		}
	}

	tower, ok := enemy.Towers[towerName]
	if !ok || tower.HP <= 0 {
		return "ERR|Invalid or destroyed tower"
	}

	// Simple attack logic - troop attacks tower
	damage := troop.ATK - tower.DEF
	if damage < 0 {
		damage = 0
	}
	tower.HP -= damage
	if tower.HP < 0 {
		tower.HP = 0
	}

	// Tower counter-attack logic - tower attacks troop
	counterDamage := tower.ATK - troop.DEF
	if counterDamage < 0 {
		counterDamage = 0
	}
	troop.HP -= counterDamage
	if troop.HP <= 0 {
		troop.HP = 0 // Đánh dấu troop đã chết/đã sử dụng
	}

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
	time.Sleep(200 * time.Millisecond)
	stateMsg := "STATE|" + formatGameState(game, username)
	sendToUser(username, stateMsg)
	sendToUser(enemyName, "STATE|"+formatGameState(game, enemyName))

	// 3. Check if the deployed troop is a Queen, and if so, activate healing ability
	if troop.Name == "Queen" {
		// Find tower with lowest HP to heal
		minHP := 999999
		var healTower *Tower
		for _, t := range player.Towers {
			if t.HP > 0 && t.HP < minHP {
				minHP = t.HP
				healTower = t
			}
		}
		// Apply healing effect if found a valid tower to heal
		if healTower != nil {
			healAmount := 300 // Heal by 300 HP
			healTower.HP += healAmount

			// Notify players about the healing
			healMsg := fmt.Sprintf("QUEEN_HEAL|%s|%s|%d|%d", username, healTower.Name, healAmount, healTower.HP)
			sendToUser(username, healMsg)
			sendToUser(enemyName, healMsg)
		}
	}

	// 4. Finally, send turn notifications with a longer delay
	time.Sleep(300 * time.Millisecond)
	// Send appropriate turn message to each player
	for playerName := range game.Players {
		if playerName == game.TurnUser {
			sendToUser(playerName, "TURN|Your turn!")
		} else {
			sendToUser(playerName, "TURN|Wait for your turn...")
		}
	}

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
			// Special display for Queen
			if tr.Name == "Queen" {
				sb.WriteString(fmt.Sprintf("    %s: Heals the tower with lowest HP by 300\n", tr.Name))
			} else {
				sb.WriteString(fmt.Sprintf("    %s: HP=%d ATK=%d DEF=%d\n",
					tr.Name, tr.HP, tr.ATK, tr.DEF))
			}
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
			})
		}
		players[uname] = &EnhancedPlayerState{
			Username: uname,
			Towers:   towers,
			Troops:   troops,
			Mana:     5,
			EXP:      progress.EXP,
			Level:    progress.Level,
			Progress: progress}
	}

	gs := &EnhancedGameState{
		RoomID:         roomID,
		Players:        players,
		Winner:         "",
		Over:           false,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(3 * time.Minute),
		AttackPatterns: make(map[string]string), // initialize attack pattern tracking
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
		// Check for end by timer or already over
		if time.Now().After(gs.EndTime) || gs.Over {
			gs.Over = true
			// Determine winner by towers remaining (ai còn nhiều tower hơn sẽ thắng)
			aliveCounts := map[string]int{}
			for uname, ps := range gs.Players {
				alive := 0
				for _, t := range ps.Towers {
					if t.HP > 0 {
						alive++
					}
				}
				aliveCounts[uname] = alive
			}
			// So sánh số tower còn sống
			var winner string
			var draw bool
			usernames := []string{}
			for uname := range aliveCounts {
				usernames = append(usernames, uname)
			}
			if len(usernames) == 2 {
				a1 := aliveCounts[usernames[0]]
				a2 := aliveCounts[usernames[1]]
				if a1 > a2 {
					winner = usernames[0]
				} else if a2 > a1 {
					winner = usernames[1]
				} else {
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
			// Send final state and GAME_END to both players
			for uname := range gs.Players {
				if v, ok := userConns.Load(uname); ok {
					if conn, ok2 := v.(net.Conn); ok2 {
						// Send final state
						state, _ := json.Marshal(gs)
						conn.Write([]byte("STATE|" + string(state) + "\n"))
						// Send GAME_END
						if gs.Winner == "DRAW" {
							conn.Write([]byte("GAME_END|Draw!\n"))
						} else if gs.Winner == uname {
							conn.Write([]byte("GAME_END|You win!\n"))
						} else {
							conn.Write([]byte("GAME_END|You lose!\n"))
						}
					}
				}
			}
			// Wait 2 seconds before cleanup to allow clients to process GAME_END
			enhancedGamesLock.Unlock()
			time.Sleep(2 * time.Second)
			enhancedGamesLock.Lock()
			delete(enhancedGames, roomID)
			roomsLock.Lock()
			delete(gameRooms, roomID)
			roomsLock.Unlock()
			enhancedGamesLock.Unlock()
			return
		}
		enhancedGamesLock.Unlock()
	}
}

// Enhanced deploy: check mana, crit, continuous
func handleEnhancedDeploy(username, troopName, targetTower string) string {
	enhancedGamesLock.Lock()
	defer enhancedGamesLock.Unlock()
	var game *EnhancedGameState
	var roomID string
	for rid, g := range enhancedGames {
		if g.Players[username] != nil {
			game = g
			roomID = rid
			break
		}
	}
	if game == nil {
		return "ERR|Not in game"
	}
	if game.Over {
		return "ERR|Game is over"
	}
	ps := game.Players[username]
	var troop *Troop
	for _, t := range ps.Troops {
		// Special case for Queen who can be used even with HP=0
		if t.Name == troopName && (t.HP > 0 || t.Name == "Queen") {
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
			spec := t
			tspec = spec
			break
		}
	}
	if ps.Mana < tspec.MANA {
		return "ERR|Not enough mana"
	}
	ps.Mana -= tspec.MANA // Find opponent
	var opp *EnhancedPlayerState
	var oppName string
	for uname, p := range game.Players {
		if uname != username {
			opp = p
			oppName = uname
			break
		}
	}

	if opp == nil {
		return "ERR|No opponent"
	}

	// Check tower attack restrictions
	if targetTower == "King" {
		// Check if at least one guard tower is destroyed before allowing King tower attack
		if opp.Towers["Guard1"].HP > 0 && opp.Towers["Guard2"].HP > 0 {
			return "ERR|Must destroy either Guard1 or Guard2 tower before attacking King"
		}
	} else if targetTower == "Guard1" || targetTower == "Guard2" {
		// Check if this is the first guard tower attack by this player
		firstGuardTower, hasPattern := game.AttackPatterns[username]

		if !hasPattern {
			// First time attacking a guard tower - record the choice
			game.AttackPatterns[username] = targetTower
		} else if firstGuardTower != targetTower {
			// Player is trying to attack the other guard tower
			if opp.Towers[firstGuardTower].HP > 0 {
				// The first guard tower is not yet destroyed, prevent switch
				return fmt.Sprintf("ERR|You must destroy %s Tower first before attacking %s Tower", firstGuardTower, targetTower)
			}
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
			// Send ATTACK_RESULT, final STATE, and GAME_END to both players
			for uname := range game.Players {
				if v, ok := userConns.Load(uname); ok {
					if conn, ok2 := v.(net.Conn); ok2 {
						conn.Write([]byte(msg + "\n"))
						state, _ := json.Marshal(game)
						conn.Write([]byte("STATE|" + string(state) + "\n"))
						if uname == username {
							conn.Write([]byte("GAME_END|You win! King destroyed.\n"))
						} else {
							conn.Write([]byte("GAME_END|You lose! King destroyed.\n"))
						}
					}
				}
			}
			// Clean up game and room
			delete(enhancedGames, roomID)
			roomsLock.Lock()
			delete(gameRooms, roomID)
			roomsLock.Unlock()
			return ""
		}
	}
	// Queen's heal
	if troop.Name == "Queen" {
		minHP := 99999
		var healTower *Tower
		for _, t := range ps.Towers {
			if t.HP > 0 && t.HP < minHP {
				minHP = t.HP
				healTower = t
			}
		}
		if healTower != nil {
			healAmount := 300
			healTower.HP += healAmount
			healMsg := fmt.Sprintf("QUEEN_HEAL|%s|%s|%d|%d", username, healTower.Name, healAmount, healTower.HP)
			if v, ok := userConns.Load(username); ok {
				if conn, ok2 := v.(net.Conn); ok2 {
					conn.Write([]byte(healMsg + "\n"))
				}
			}
			if v, ok := userConns.Load(oppName); ok {
				if conn, ok2 := v.(net.Conn); ok2 {
					conn.Write([]byte(healMsg + "\n"))
				}
			}
		}
	}
	// Send ATTACK_RESULT and updated STATE to both players
	for uname := range game.Players {
		if v, ok := userConns.Load(uname); ok {
			if conn, ok2 := v.(net.Conn); ok2 {
				conn.Write([]byte(msg + "\n"))
				state, _ := json.Marshal(game)
				conn.Write([]byte("STATE|" + string(state) + "\n"))
			}
		}
	}
	return ""
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

// Helper function to handle a player exiting the game
func handlePlayerExit(username string) {
	// First check if the player is in a simple game
	gamesLock.Lock()
	var gameFound *GameState
	var gameRoomID string
	var opponent string

	for roomID, g := range games {
		if _, ok := g.Players[username]; ok {
			gameFound = g
			gameRoomID = roomID

			// Find opponent name
			for uname := range g.Players {
				if uname != username {
					opponent = uname
					break
				}
			}
			break
		}
	}

	if gameFound != nil {
		// Notify opponent that this player has left
		if opponent != "" {
			// Send GAME_END to opponent to return them to the menu
			sendToUser(opponent, "GAME_END|Your opponent has left the game")
		}

		// Remove the game
		delete(games, gameRoomID)
		gamesLock.Unlock()
		return
	}
	gamesLock.Unlock()

	// Check if player is in an enhanced game
	enhancedGamesLock.Lock()
	var enhancedGameFound *EnhancedGameState
	var enhancedGameRoomID string
	opponent = ""

	for roomID, g := range enhancedGames {
		if _, ok := g.Players[username]; ok {
			enhancedGameFound = g
			enhancedGameRoomID = roomID

			// Find opponent name
			for uname := range g.Players {
				if uname != username {
					opponent = uname
					break
				}
			}
			break
		}
	}

	if enhancedGameFound != nil {
		// Notify opponent that this player has left
		if opponent != "" {
			// Send GAME_END to opponent to return them to the menu
			sendToUser(opponent, "GAME_END|Your opponent has left the game")
		}

		// Remove the game
		delete(enhancedGames, enhancedGameRoomID)
		// Remove the room from gameRooms to allow new games with same name
		roomsLock.Lock()
		delete(gameRooms, enhancedGameRoomID)
		roomsLock.Unlock()
	}
	enhancedGamesLock.Unlock()
}
