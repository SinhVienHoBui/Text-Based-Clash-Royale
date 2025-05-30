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
	fmt.Println("TCR Server starting...") // Print server start message
	ln, err := net.Listen("tcp", ":9000") // Listen for TCP connections on port 9000
	if err != nil {
		fmt.Println("Error starting server:", err) // Print error if cannot start
		os.Exit(1)                                 // Exit if error
	}
	defer ln.Close()                         // Ensure listener is closed on exit
	fmt.Println("Server listening on :9000") // Print listening message
	for {
		conn, err := ln.Accept() // Accept new connection
		if err != nil {
			fmt.Println("Accept error:", err) // Print error if accept fails
			continue                          // Continue to next iteration
		}
		go handleConnection(conn) // Handle connection in a new goroutine
	}
}

// handleConnection handles all communication with a single client connection
// conn: the TCP connection to the client
func handleConnection(conn net.Conn) {
	defer conn.Close() // Ensure connection is closed when function exits
	// send is a helper function to send a message to the client, always appending a newline
	send := func(msg string) { conn.Write([]byte(msg + "\n")) }
	// scanner reads lines from the client connection
	scanner := bufio.NewScanner(conn)
	var currentUser *User      // Track the currently logged-in user (if any)
	var currentUsername string // Track the username of the current user
	for scanner.Scan() {       // Main loop: read each line from client
		line := scanner.Text()                // Read a line from client
		parts := strings.SplitN(line, "|", 3) // Split command by |
		cmd := strings.ToUpper(parts[0])      // Get command type (always uppercase)
		switch cmd {
		case "LOGIN":
			if len(parts) < 3 {
				send("ERR|Usage: LOGIN|username|password") // Not enough arguments
				continue
			}
			user := authenticate(parts[1], parts[2]) // Check credentials
			if user != nil {
				currentUser = user                   // Save user struct
				currentUsername = user.Username      // Save username
				userConns.Store(user.Username, conn) // Store connection in global map
				send("ACK|Login successful")         // Notify client
			} else {
				send("ERR|Invalid credentials") // Wrong username/password
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
			mode := "SIMPLE" // Default mode
			if len(parts) > 1 {
				mode = strings.ToUpper(parts[1]) // Use provided mode if present
			}
			roomID := createGameRoom(currentUser.Username, mode) // Create new room
			send("ACK|GAME_CREATED|" + roomID)                   // Notify client
			// If ENHANCED mode, wait for guest and start game automatically
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
			continue // Skip rest of loop
		case "LIST_GAMES":
			if currentUser == nil {
				send("ERR|Login first")
				continue
			}
			list := listGameRooms() // Get available rooms
			send("GAMES|" + list)
		case "JOIN_GAME":
			if currentUser == nil || len(parts) < 2 {
				send("ERR|Usage: JOIN_GAME|room_id")
				continue
			}
			ok := joinGameRoom(parts[1], currentUser.Username)
			if ok {
				room := gameRooms[parts[1]]
				if room != nil && room.Host != "" && room.Guest != "" {
					if room.Mode == "ENHANCED" {
						startEnhancedGame(parts[1])
						for _, uname := range []string{room.Host, room.Guest} {
							if v, ok := userConns.Load(uname); ok {
								if conn, ok2 := v.(net.Conn); ok2 {
									conn.Write([]byte("ACK|GAME_STARTED\n"))
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
			// Check if player is in an enhanced game
			enhancedGamesLock.Lock()
			inEnhancedGame := false
			for _, g := range enhancedGames {
				if _, ok := g.Players[currentUsername]; ok {
					inEnhancedGame = true
					break
				}
			}
			enhancedGamesLock.Unlock()
			if inEnhancedGame {
				response := handleEnhancedDeploy(currentUsername, parts[1], parts[2])
				if response != "" {
					send(response)
				}
			} else {
				send(handleDeploy(currentUsername, parts[1], parts[2]))
			}
		case "STATE":
			if currentUser == nil {
				send("ERR|Login first")
				continue
			}
			enhancedGamesLock.Lock()
			inEnhancedGame := false
			for _, g := range enhancedGames {
				if _, ok := g.Players[currentUsername]; ok {
					inEnhancedGame = true
					break
				}
			}
			enhancedGamesLock.Unlock()
			if inEnhancedGame {
				send(getEnhancedGameState(currentUsername))
			} else {
				send(getGameState(currentUsername))
			}
		case "EXIT_GAME":
			if currentUser == nil {
				send("ERR|Login first")
				continue
			}
			handlePlayerExit(currentUser.Username)
			send("GAME_END|You have exited the game")
		case "BUY":
			if currentUser == nil {
				send("ERR|Login first")
				continue
			}
			if len(parts) < 2 {
				send("ERR|Usage: BUY|troop_name")
				continue
			}
			response := handleEnhancedBuy(currentUsername, parts[1])
			send(response)
		default:
			send("ERR|Unknown command")
		}
	}
	if currentUsername != "" {
		userConns.Delete(currentUsername) // Remove user from active connections on disconnect
	}
}

// Global map for user connections
var userConns sync.Map // username -> net.Conn

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

// createGameRoom creates a new game room with the given host and mode (SIMPLE or ENHANCED)
// Returns the room ID string
func createGameRoom(host, mode string) string {
	roomsLock.Lock()                              // Lock the rooms map for thread safety
	defer roomsLock.Unlock()                      // Ensure unlock after function
	id := fmt.Sprintf("room%d", len(gameRooms)+1) // Generate a unique room ID
	if mode == "ENHANCED" {
		gameRooms[id] = &GameRoom{ID: id, Host: host, Started: false, Mode: "ENHANCED"} // Create enhanced room
	} else {
		gameRooms[id] = &GameRoom{ID: id, Host: host, Started: false, Mode: "SIMPLE"} // Create simple room
	}
	return id // Return the new room ID
}

// listGameRooms returns a comma-separated string of all available (not started) rooms
func listGameRooms() string {
	roomsLock.Lock() // Lock for thread safety
	defer roomsLock.Unlock()
	var ids []string // Slice to hold room info
	for id, room := range gameRooms {
		if !room.Started && room.Guest == "" {
			ids = append(ids, id+":"+room.Host) // Format: roomID:host
		}
	}
	return strings.Join(ids, ",") // Return as comma-separated string
}

// joinGameRoom allows a guest to join a room by ID
// Returns true if successful, false otherwise
func joinGameRoom(id, guest string) bool {
	roomsLock.Lock() // Lock for thread safety
	defer roomsLock.Unlock()
	room, ok := gameRooms[id] // Find the room
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
		return startEnhancedGame(roomID) // Start enhanced game if needed
	}
	// Only start if both players are present
	if room.Host == "" || room.Guest == "" {
		return false
	}
	if _, exists := games[roomID]; exists {
		// If game already exists, do not re-initialize
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
	rand.Seed(time.Now().UnixNano()) // Seed random for troop selection
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
	}
	// Randomly pick who starts
	turnIdx := rand.Intn(2)
	turnUser := room.Host
	if turnIdx == 1 {
		turnUser = room.Guest
	}
	players[turnUser].Turn = true // Set turn for starting player
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

// countAliveTroops returns the number of alive troops (HP > 0) for a player
func countAliveTroops(ps *PlayerState) int {
	c := 0
	for _, tr := range ps.Troops {
		if tr.HP > 0 {
			c++
		}
	}
	return c
}

// countAliveTowers returns the number of alive towers (HP > 0)
func countAliveTowers(towers map[string]*Tower) int {
	cnt := 0
	for _, tw := range towers {
		if tw.HP > 0 {
			cnt++
		}
	}
	return cnt
}

// sumTowerHP returns the total HP of all towers
func sumTowerHP(towers map[string]*Tower) int {
	total := 0
	for _, tw := range towers {
		total += tw.HP
	}
	return total
}

// handleDeploy processes a deploy command from a player in simple mode
// username: the player making the move
// troopName: the troop to deploy
// towerName: the target tower
// Returns a string message to send back to the client
func handleDeploy(username, troopName, towerName string) string {
	gamesLock.Lock() // Lock the games map for thread safety
	defer gamesLock.Unlock()
	var game *GameState
	for _, g := range games {
		if _, ok := g.Players[username]; ok {
			game = g
			break
		}
	}
	if game == nil || game.Over {
		return "ERR|No active game" // No game found or already over
	}
	if game.TurnUser != username {
		return "ERR|Not your turn" // Not this player's turn
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
		return "ERR|Invalid or dead troop" // Troop not found or dead
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
		// Must destroy at least one guard tower before attacking King
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
				return fmt.Sprintf("ERR|You must destroy %s Tower first before attacking %s Tower", firstGuardTower, towerName)
			}
		}
	}
	tower, ok := enemy.Towers[towerName]
	if !ok || tower.HP <= 0 {
		return "ERR|Invalid or destroyed tower" // Tower not found or already destroyed
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

	// Check if all troops are dead for either player
	if countAliveTroops(player) == 0 || countAliveTroops(enemy) == 0 {
		// Count alive towers for both players
		aliveP := countAliveTowers(player.Towers)
		aliveE := countAliveTowers(enemy.Towers)
		if aliveP != aliveE {
			// Whoever has more towers alive wins
			var winner, loser string
			if aliveP > aliveE {
				winner = username
				loser = enemyName
			} else {
				winner = enemyName
				loser = username
			}
			game.Over = true
			game.Winner = winner
			sendToUser(winner, fmt.Sprintf("GAME_END|You win! You have more towers alive (%d vs %d).", aliveP, aliveE))
			sendToUser(loser, fmt.Sprintf("GAME_END|You lose! Fewer towers alive (%d vs %d).", aliveE, aliveP))
		} else {
			// If tower count is equal, compare total HP
			hpP := sumTowerHP(player.Towers)
			hpE := sumTowerHP(enemy.Towers)
			switch {
			case hpP > hpE:
				game.Over = true
				game.Winner = username
				sendToUser(username, "GAME_END|You win! Equal tower count but higher total HP.")
				sendToUser(enemyName, "GAME_END|You lose! Equal tower count but lower total HP.")
			case hpE > hpP:
				game.Over = true
				game.Winner = enemyName
				sendToUser(enemyName, "GAME_END|You win! Equal tower count but higher total HP.")
				sendToUser(username, "GAME_END|You lose! Equal tower count but lower total HP.")
			default:
				game.Over = true
				sendToUser(username, "GAME_END|Draw! Equal tower count and equal total HP.")
				sendToUser(enemyName, "GAME_END|Draw! Equal tower count and equal total HP.")
			}
		}
		return "STATE|" + formatGameState(game, username)
	}

	// Tower counter-attack logic - tower attacks troop
	counterDamage := tower.ATK - troop.DEF
	if counterDamage < 0 {
		counterDamage = 0
	}
	troop.HP -= counterDamage
	if troop.HP <= 0 {
		troop.HP = 0 // Mark troop as dead/used
	}

	// Check for win by King destroyed
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
	// 2. Then send updated state with a small delay to both players
	time.Sleep(200 * time.Millisecond)
	stateMsg = "STATE|" + formatGameState(game, username)
	sendToUser(username, stateMsg)
	sendToUser(enemyName, "STATE|"+formatGameState(game, enemyName))

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

// Enhanced BUY: mua troop tốn mana, thêm vào danh sách troops đã mua
func handleEnhancedBuy(username, troopName string) string {
	enhancedGamesLock.Lock()
	defer enhancedGamesLock.Unlock()
	var game *EnhancedGameState
	for _, g := range enhancedGames {
		if g.Players[username] != nil {
			game = g
			break
		}
	}
	if game == nil {
		return "ERR|Not in enhanced game"
	}
	ps := game.Players[username]
	// Tìm troop spec
	var tspec TroopSpec
	found := false
	for _, t := range troopSpecs {
		if t.Name == troopName {
			tspec = t
			found = true
			break
		}
	}
	if !found {
		return "ERR|No such troop"
	}
	if ps.Mana < tspec.MANA {
		return "ERR|Not enough mana"
	}
	ps.Mana -= tspec.MANA
	// Stat scaling by user level
	mult := 1.0 + 0.1*float64(ps.Level-1)
	ps.Troops = append(ps.Troops, &Troop{
		Name:  tspec.Name,
		HP:    int(float64(tspec.HP) * mult),
		ATK:   int(float64(tspec.ATK) * mult),
		DEF:   int(float64(tspec.DEF) * mult),
		Owner: username,
	})
	return "ACK|Buy successful"
}

// sendToUser sends a message to a user if they are connected
// username: the username to send to
// msg: the message to send
func sendToUser(username, msg string) {
	if v, ok := userConns.Load(username); ok { // Check if user is in the map
		if conn, ok2 := v.(net.Conn); ok2 { // Type assert to net.Conn
			conn.Write([]byte(msg + "\n")) // Write message with newline
		}
	}
}

// getGameState returns the current game state for a user as a string
func getGameState(username string) string {
	gamesLock.Lock() // Lock for thread safety
	defer gamesLock.Unlock()
	for _, g := range games {
		if g.Players[username] != nil {
			return "STATE|" + formatGameState(g, username) // Return formatted state
		}
	}
	return "ERR|Not in game" // Not found
}

// formatGameState formats the game state for display to a user
// g: pointer to GameState
// username: the user requesting the state
func formatGameState(g *GameState, username string) string {
	var sb strings.Builder                              // Use a string builder for efficiency
	sb.WriteString(fmt.Sprintf("Room: %s\n", g.RoomID)) // Room ID
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

// ternary is a helper function for inline if-else
func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

// loadSpecs loads tower and troop specs from the JSON file
func loadSpecs() {
	data, err := ioutil.ReadFile(specsFile) // Read the file
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
	towerSpecs = specs.Towers // Assign loaded towers
	troopSpecs = specs.Troops // Assign loaded troops
}

// init is called automatically before main
func init() {
	loadSpecs() // Load specs at startup
}

// Load/save player progress (exp, level, etc.)
// Hàm loadProgress lấy thông tin tiến trình (level, exp, ...) của user từ file JSON
func loadProgress(username string) *PlayerProgress {
	usersLock.Lock()                           // Khóa để tránh race condition khi đọc file
	defer usersLock.Unlock()                   // Mở khóa khi hàm kết thúc
	data, err := ioutil.ReadFile(progressFile) // Đọc file tiến trình (users.json)
	if err != nil {
		// Nếu lỗi (chưa có file), trả về tiến trình mặc định (level 1, chưa có nâng cấp tower/troop)
		return &PlayerProgress{Username: username, Level: 1, TowerLv: map[string]int{}, TroopLv: map[string]int{}}
	}
	var users UsersData              // Tạo biến lưu danh sách user
	_ = json.Unmarshal(data, &users) // Parse JSON thành struct UsersData
	for _, u := range users.Users {
		if u.Username == username {
			// Nếu tìm thấy user, trả về tiến trình với EXP, Level hiện tại
			return &PlayerProgress{Username: u.Username, EXP: u.EXP, Level: u.Level, TowerLv: map[string]int{}, TroopLv: map[string]int{}}
		}
	}
	// Nếu không tìm thấy user, trả về tiến trình mặc định
	return &PlayerProgress{Username: username, Level: 1, TowerLv: map[string]int{}, TroopLv: map[string]int{}}
}

// Hàm saveProgress lưu lại tiến trình (level, exp, ...) của user vào file JSON
func saveProgress(progress *PlayerProgress) {
	usersLock.Lock() // Khóa để tránh race condition khi ghi file
	defer usersLock.Unlock()
	data, err := ioutil.ReadFile(progressFile) // Đọc file tiến trình
	if err != nil {
		return // Nếu lỗi thì bỏ qua
	}
	var users UsersData
	_ = json.Unmarshal(data, &users) // Parse JSON thành struct
	for i, u := range users.Users {
		if u.Username == progress.Username {
			// Nếu tìm thấy user, cập nhật EXP và Level
			users.Users[i].EXP = progress.EXP
			users.Users[i].Level = progress.Level
		}
	}
	out, _ := json.MarshalIndent(users, "", "  ") // Chuyển lại thành JSON
	_ = ioutil.WriteFile(progressFile, out, 0644) // Ghi ra file
}

// Enhanced game start: continuous, mana, exp, timer
// Khởi tạo game nâng cao (enhanced mode), phát quân, tower, mana, exp, timer cho mỗi user
func startEnhancedGame(roomID string) bool {
	enhancedGamesLock.Lock() // Khóa để tránh race condition
	defer enhancedGamesLock.Unlock()
	room, ok := gameRooms[roomID]
	if !ok || !room.Started {
		return false // Nếu phòng không tồn tại hoặc chưa start thì trả về false
	}
	if room.Host == "" || room.Guest == "" {
		return false // Nếu thiếu người thì không start
	}
	if _, exists := enhancedGames[roomID]; exists {
		return false // Nếu game đã tồn tại thì không khởi tạo lại
	}
	players := map[string]*EnhancedPlayerState{}
	for _, uname := range []string{room.Host, room.Guest} {
		progress := loadProgress(uname) // Lấy tiến trình user
		level := 1
		if progress != nil {
			level = progress.Level // Lấy level hiện tại
		}
		mult := 1.0 + 0.1*float64(level-1) // Tính hệ số nhân theo level
		towers := map[string]*Tower{}
		for _, v := range towerSpecs {
			lv := progress.TowerLv[v.Name]
			if lv == 0 {
				lv = 1
			}
			// Nhân chỉ số tower theo level
			towers[v.Name] = &Tower{
				Name: v.Name,
				HP:   int(float64(v.HP) * mult),
				ATK:  int(float64(v.ATK) * mult),
				DEF:  int(float64(v.DEF) * mult),
			}
		}
		// Phát 3 troops ngẫu nhiên đầu game
		availableTroopNames := make([]string, 0, len(troopSpecs))
		for _, t := range troopSpecs {
			availableTroopNames = append(availableTroopNames, t.Name)
		}
		rand.Seed(time.Now().UnixNano())
		rand.Shuffle(len(availableTroopNames), func(i, j int) {
			availableTroopNames[i], availableTroopNames[j] = availableTroopNames[j], availableTroopNames[i]
		})
		selected := availableTroopNames[:3]
		troops := []*Troop{}
		for _, tn := range selected {
			var tspec TroopSpec
			for _, t := range troopSpecs {
				if t.Name == tn {
					tspec = t
					break
				}
			}
			troops = append(troops, &Troop{
				Name:  tspec.Name,
				HP:    int(float64(tspec.HP) * mult),
				ATK:   int(float64(tspec.ATK) * mult),
				DEF:   int(float64(tspec.DEF) * mult),
				Owner: uname,
			})
		}
		players[uname] = &EnhancedPlayerState{
			Username: uname,
			Towers:   towers,
			Troops:   troops,
			Mana:     5, // Mỗi user bắt đầu với 5 mana
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
		EndTime:        time.Now().Add(3 * time.Minute), // Game kéo dài 3 phút
		AttackPatterns: make(map[string]string),
	}
	enhancedGames[roomID] = gs  // Lưu game vào map
	go enhancedGameLoop(roomID) // Chạy goroutine quản lý game loop
	return true
}

// Enhanced game loop: mana regen, timer, end conditions
// Goroutine này chạy liên tục để hồi mana, kiểm tra hết giờ, tính thắng/thua, cập nhật exp/level
func enhancedGameLoop(roomID string) {
	for {
		time.Sleep(1 * time.Second) // Mỗi giây lặp lại
		enhancedGamesLock.Lock()
		gs, ok := enhancedGames[roomID]
		if !ok || gs.Over {
			enhancedGamesLock.Unlock()
			return // Nếu game đã kết thúc thì dừng
		}
		for _, ps := range gs.Players {
			if ps.Mana < 10 {
				ps.Mana++ // Hồi mana mỗi giây, tối đa 10
			}
		}
		// Kiểm tra hết giờ hoặc game đã kết thúc
		if time.Now().After(gs.EndTime) || gs.Over {
			gs.Over = true
			// Tính số tower còn sống của mỗi người
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
			// So sánh số tower còn sống để xác định thắng/thua/hòa
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
			// Cộng EXP cho người chơi
			for uname, ps := range gs.Players {
				oppName := ""
				for n := range gs.Players {
					if n != uname {
						oppName = n
						break
					}
				}
				oppLv := 1
				if oppName != "" {
					oppLv = gs.Players[oppName].Level
				}
				if gs.Winner == "DRAW" {
					ps.EXP += 2 * oppLv // Hòa thì cộng ít exp
				} else if gs.Winner == uname {
					ps.EXP += 5 * oppLv // Thắng thì cộng nhiều exp
				}
				// Tăng level nếu đủ exp
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
			// Gửi trạng thái cuối cùng và GAME_END cho cả hai người chơi
			for uname := range gs.Players {
				if v, ok := userConns.Load(uname); ok {
					if conn, ok2 := v.(net.Conn); ok2 {
						// Gửi trạng thái cuối cùng
						state, _ := json.Marshal(gs)
						conn.Write([]byte("STATE|" + string(state) + "\n"))
						// Gửi GAME_END
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
			// Đợi 2 giây rồi xóa game khỏi bộ nhớ
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
	// Lấy troop đã mua (không tạo mới, không trừ mana khi deploy)
	var troop *Troop
	for _, t := range ps.Troops {
		if t.Name == troopName {
			if t.Name == "Queen" {
				// Queen luôn luôn deploy được, không quan tâm HP
				troop = t
				break
			} else if t.HP > 0 {
				troop = t
				break
			}
		}
	}
	if troop == nil {
		return "ERR|No such troop or dead"
	}
	// Không kiểm tra/trừ mana ở đây nữa
	// Find opponent
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
	// CRIT logic (tower attacks troop)
	critChance := 0.0
	for _, t := range towerSpecs {
		if t.Name == targetTower {
			critChance = t.CRIT
			break
		}
	}
	crit := rand.Float64() < critChance
	// Troop attacks tower
	atk := troop.ATK
	dmg := atk - tower.DEF
	if dmg < 0 {
		dmg = 0
	}
	tower.HP -= dmg
	// Tower phản công troop (có CRIT)
	counterATK := tower.ATK
	if crit {
		counterATK = int(float64(counterATK) * 1.2)
	}
	counterDmg := counterATK - troop.DEF
	if counterDmg < 0 {
		counterDmg = 0
	}
	troop.HP -= counterDmg
	if troop.HP < 0 {
		troop.HP = 0
	}
	msg := fmt.Sprintf("ATTACK_RESULT|%s|%s|%d|%d|TOWER_HIT:%d|CRIT:%v|TROOP_HP:%d", troop.Name, tower.Name, dmg, tower.HP, counterDmg, crit, troop.HP)
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
			delete(enhancedGames, roomID)
			roomsLock.Lock()
			delete(gameRooms, roomID)
			roomsLock.Unlock()
			return "ACK|Deploy successful"
		}
	}
	// Queen's heal (giống simple: chỉ heal cho Guard1 hoặc Guard2 nếu còn sống, chọn tower có HP thấp nhất)
	if troop.Name == "Queen" {
		minHP := 99999
		var healTower *Tower
		for _, tname := range []string{"Guard1", "Guard2"} {
			t := ps.Towers[tname]
			if t.HP > 0 && t.HP < minHP {
				minHP = t.HP
				healTower = t
			}
		}
		if healTower != nil {
			healAmount := 300
			before := healTower.HP
			healTower.HP = before + healAmount // Always add 300, no cap
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
	return "ACK|Deploy successful"
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
