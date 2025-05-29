package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// Main entry point for the client application
func main() {
	// Print a startup message
	fmt.Println("TCR Client starting...")
	// Connect to the server at localhost:9000 using TCP
	conn, err := net.Dial("tcp", "localhost:9000")
	if err != nil {
		// If connection fails, print error and exit
		fmt.Println("Unable to connect to server:", err)
		os.Exit(1)
	}
	// Ensure the connection is closed when main exits
	defer conn.Close()
	// Print successful connection message
	fmt.Println("Connected to server.")
	// Create a buffered reader for user input from stdin
	reader := bufio.NewReader(os.Stdin)
	// Create a scanner to read messages from the server
	serverScanner := bufio.NewScanner(conn)
	// Login/Register loop
	for {
		// Show login/register menu
		fmt.Println("1. Login\n2. Register\nChoose:")
		// Read user choice
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)
		if choice == "1" {
			// Prompt for username
			fmt.Print("Username: ")
			user, _ := reader.ReadString('\n')
			// Prompt for password
			fmt.Print("Password: ")
			pass, _ := reader.ReadString('\n')
			// Format login message
			msg := fmt.Sprintf("LOGIN|%s|%s", strings.TrimSpace(user), strings.TrimSpace(pass))
			// Send login message to server
			conn.Write([]byte(msg + "\n"))
		} else if choice == "2" {
			// Prompt for username
			fmt.Print("Username: ")
			user, _ := reader.ReadString('\n')
			// Prompt for password
			fmt.Print("Password: ")
			pass, _ := reader.ReadString('\n')
			// Format register message
			msg := fmt.Sprintf("REGISTER|%s|%s", strings.TrimSpace(user), strings.TrimSpace(pass))
			// Send register message to server
			conn.Write([]byte(msg + "\n"))
		} else {
			// Invalid choice, prompt again
			fmt.Println("Invalid choice.")
			continue
		}
		// Wait for server response (ACK or ERR)
		for serverScanner.Scan() {
			msg := serverScanner.Text()
			fmt.Println(msg)
			if strings.HasPrefix(msg, "ACK|Login successful") || strings.HasPrefix(msg, "ACK|Register successful") {
				// Successful login/register, break loop
				break
			} else if strings.HasPrefix(msg, "ERR|") {
				// Error, exit client
				os.Exit(1)
			}
		}
		break
	} // end login/register
	// After login/register, allow game creation/joining
	for {
		// Show main menu
		fmt.Println("1. Create Game\n2. List/Join Game\n3. Exit\nChoose:")
		// Read user choice
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)
		if choice == "3" {
			// Exit client
			fmt.Println("Exiting client. Goodbye!")
			os.Exit(0)
		} else if choice == "1" {
			// Prompt for game mode
			fmt.Println("Select game mode: 1. Simple  2. Enhanced")
			mode, _ := reader.ReadString('\n')
			mode = strings.TrimSpace(mode)
			if mode == "2" {
				// Create enhanced game
				conn.Write([]byte("CREATE_GAME|ENHANCED\n"))
			} else {
				// Create simple game
				conn.Write([]byte("CREATE_GAME|SIMPLE\n"))
			}
			// Wait for game to start
			waitForGameStart(serverScanner, conn, mode)
		} else if choice == "2" {
			// List available games
			conn.Write([]byte("LIST_GAMES\n"))
			var roomsLine string
			for serverScanner.Scan() {
				msg := serverScanner.Text()
				if strings.HasPrefix(msg, "GAMES|") {
					roomsLine = msg[6:]
					break
				} else if strings.Contains(msg, "No available rooms") {
					fmt.Println("No available rooms to join. Returning to menu.")
					roomsLine = ""
					break
				} else {
					fmt.Println(msg)
				}
			}
			if roomsLine == "" {
				continue
			}
			// Parse and display available rooms
			rooms := strings.Split(roomsLine, ",")
			fmt.Println("Available rooms:")
			for _, r := range rooms {
				if r == "" {
					continue
				}
				parts := strings.SplitN(r, ":", 2)
				if len(parts) == 2 {
					fmt.Printf("- Room ID: %s (Host: %s)\n", parts[0], parts[1])
				} else {
					fmt.Printf("- Room ID: %s\n", r)
				}
			}
			// Prompt user to enter room id to join
			fmt.Print("Enter room id to join: ")
			room, _ := reader.ReadString('\n')
			room = strings.TrimSpace(room)
			if room != "" {
				// Send join request to server
				conn.Write([]byte("JOIN_GAME|" + room + "\n"))
				waitForGameStart(serverScanner, conn, "")
			}
		} else {
			// Invalid choice, prompt again
			fmt.Println("Invalid choice.")
		}
	}
}

// Only one scanner reads from server, and all game logic is handled here
func waitForGameStart(scanner *bufio.Scanner, conn net.Conn, mode string) {
	// Reset the enhanced input goroutine for every new game
	staticEnhancedInputOnce = sync.Once{}   // Reset for every new game
	enhancedInputStop = make(chan struct{}) // Reset stop channel for each game
	fmt.Println("[Waiting for game to start...]")
	// Listen for server messages until game starts or error
	for scanner.Scan() {
		msg := scanner.Text()
		fmt.Println(msg)
		if strings.HasPrefix(msg, "ACK|GAME_STARTED") {
			fmt.Println("[Game started successfully!]")
			if mode != "2" && mode != "ENHANCED" {
				fmt.Println("[Waiting for turn info...]")
			}
			// Start the main game loop for this mode
			listenTurnLoop(scanner, conn, mode)
			return
		}
		if strings.HasPrefix(msg, "ERR|") {
			fmt.Println("[Error starting game]")
			os.Exit(1)
		}
	}
}

func listenTurnLoop(scanner *bufio.Scanner, conn net.Conn, mode string) {
	myTurn := false                                   // Track if it's the player's turn
	currentState := ""                                // Store the current game state as a string
	waitingForInput := false                          // Prevent multiple input goroutines
	isEnhanced := (mode == "2" || mode == "ENHANCED") // Detect enhanced mode
	// Nếu mode rỗng (join phòng), tự động nhận diện ENHANCED nếu nhận được STATE|{...json...}
	var enhancedDetected bool
	if isEnhanced {
		staticEnhancedInputOnce.Do(func() {
			go enhancedInputLoop(conn)
		})
	}
	for scanner.Scan() {
		msg := scanner.Text() // Read message from server

		if strings.HasPrefix(msg, "STATE|") {
			jsonStr := msg[6:]
			var state EnhancedGameState
			if !isEnhanced && !enhancedDetected {
				if strings.HasPrefix(strings.TrimSpace(jsonStr), "{") {
					enhancedDetected = true
					staticEnhancedInputOnce.Do(func() {
						go enhancedInputLoop(conn)
					})
				}
			}
			if isEnhanced || enhancedDetected {
				err := json.Unmarshal([]byte(jsonStr), &state)
				if err == nil {
					printEnhancedState(&state, conn)
				} else {
					fmt.Println("[ENHANCED]", jsonStr)
				}
			} else {
				fmt.Println("[Game State Update]")
				currentState = msg[6:]
				fmt.Println(currentState)
			}
		} else if strings.HasPrefix(msg, "ATTACK_RESULT|") {
			fmt.Println("[Attack Result]")
			fmt.Println(msg)
		} else if strings.HasPrefix(msg, "QUEEN_HEAL|") {
			parts := strings.Split(msg, "|")
			if len(parts) >= 5 {
				playerName := parts[1]
				towerName := parts[2]
				healAmount := parts[3]
				newHP := parts[4]
				fmt.Printf("[Queen's Healing] %s's Queen healed %s tower for %s HP (new HP: %s)\n",
					playerName, towerName, healAmount, newHP)
			} else {
				fmt.Println("[Queen's Healing]", msg)
			}
		} else if strings.HasPrefix(msg, "GAME_END|") {
			fmt.Println("[Game End]")
			fmt.Println(msg)

			// Signal enhanced input loop to stop and wait for it to exit
			if isEnhanced || enhancedDetected {
				if enhancedInputStop != nil {
					close(enhancedInputStop)
					time.Sleep(300 * time.Millisecond)
				}
				// Flush any leftover input from stdin to avoid eating the first menu input
				reader := bufio.NewReader(os.Stdin)
				reader.ReadString('\n')
			}

			// Reset các biến trạng thái để tránh enhanced goroutine còn hoạt động
			isEnhanced = false
			enhancedDetected = false
			staticEnhancedInputOnce = sync.Once{}
			enhancedInputStop = make(chan struct{})

			fmt.Println("Returning to main menu...")
			return
		} else if !isEnhanced && !enhancedDetected && strings.HasPrefix(msg, "TURN|Your turn!") {
			myTurn = true
			fmt.Println("[Turn Update] Your turn!")
			if !waitingForInput {
				waitingForInput = true
				go func() {
					inGameLoop(scanner, conn, mode, myTurn, currentState)
					waitingForInput = false
				}()
			}
		} else if !isEnhanced && !enhancedDetected && strings.HasPrefix(msg, "TURN|Wait for your turn...") {
			myTurn = false
			fmt.Println("[Turn Update] Wait for your turn...")
			fmt.Println("[Đối thủ đã thực hiện xong lượt đi]")
			fmt.Println("[Chờ đến lượt của bạn...]")
		} else if strings.HasPrefix(msg, "ERR|") {
			fmt.Println("[Error]", msg[4:])
			if myTurn && !waitingForInput {
				waitingForInput = true
				go func() {
					fmt.Println("[Error detected! Please try again with a valid input]")
					if strings.Contains(msg, "Must destroy either Guard1 or Guard2") {
						fmt.Println("[Reminder: You must destroy EITHER Guard1 OR Guard2 Tower before attacking King]")
					} else if strings.Contains(msg, "You must destroy") && strings.Contains(msg, "Tower first before attacking") {
						fmt.Println("[Reminder: Once you start attacking a Guard Tower, you must destroy it completely before attacking the other Guard Tower]")
					}
					inGameLoop(scanner, conn, mode, myTurn, currentState)
					waitingForInput = false
				}()
			}
		} else if strings.HasPrefix(msg, "ACK|") {
			if myTurn {
				fmt.Println("[Server]", msg[4:])
			}
		} else {
			fmt.Println(msg)
		}
	}
}

var staticEnhancedInputOnce sync.Once
var enhancedInputStop chan struct{} // Channel to signal enhanced input loop to stop

type EnhancedGameState struct {
	RoomID    string
	Players   map[string]EnhancedPlayer `json:"Players"`
	Winner    string
	Over      bool
	StartTime string
	EndTime   string
}
type EnhancedPlayer struct {
	Username string
	Towers   map[string]Tower
	Troops   []Troop
	Mana     int
	EXP      int
	Level    int
	Progress struct {
		Username string `json:"username"`
		EXP      int    `json:"exp"`
		Level    int    `json:"level"`
	}
}

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
	Owner string
}

// printEnhancedState prints the current enhanced game state in a user-friendly format
// state: pointer to the EnhancedGameState struct containing all game info
// conn: the network connection to the server (not used for printing, but may be used for future extensions)
func printEnhancedState(state *EnhancedGameState, conn net.Conn) {
	fmt.Println("\n========== ENHANCED GAME STATE ==========") // Print header
	fmt.Printf("Room: %s\n", state.RoomID)                     // Print room ID
	for uname, p := range state.Players {                      // Loop through all players in the game
		fmt.Printf("Player: %s (Level %d, EXP %d, Mana %d)\n", uname, p.Level, p.EXP, p.Mana) // Print player info
		fmt.Println("  Towers:")
		for _, t := range []string{"Guard1", "Guard2", "King"} { // Always print towers in this order
			tower := p.Towers[t]
			if tower.HP <= 0 {
				continue // Skip dead towers
			}
			crit := ""
			if t == "King" {
				crit = "CRIT: 10% (Tower attacks troop)" // King tower has higher crit chance
			} else {
				crit = "CRIT: 5% (Tower attacks troop)" // Guard towers have lower crit chance
			}
			fmt.Printf("    %s: HP=%d ATK=%d DEF=%d %s\n", tower.Name, tower.HP, tower.ATK, tower.DEF, crit)
		}
		fmt.Println("  Troops you own:")
		for _, tr := range p.Troops {
			if tr.Name == "Queen" {
				fmt.Printf("    %s: Special (Heal) - always available\n", tr.Name) // Queen's special ability
			} else if tr.HP > 0 {
				fmt.Printf("    %s: HP=%d ATK=%d DEF=%d\n", tr.Name, tr.HP, tr.ATK, tr.DEF) // Print troop stats
			}
			// Dead troops are hidden from UI
		}
	}
	// Print available troops to buy (static info)
	fmt.Println("-----------------------------------------")
	fmt.Println("Available troops to buy:")
	fmt.Println("  Pawn   (HP: 50,  ATK: 150, DEF: 100, MANA: 3)")
	fmt.Println("  Bishop (HP: 100, ATK: 200, DEF: 150, MANA: 4)")
	fmt.Println("  Rook   (HP: 250, ATK: 200, DEF: 200, MANA: 5)")
	fmt.Println("  Knight (HP: 200, ATK: 300, DEF: 150, MANA: 5)")
	fmt.Println("  Prince (HP: 500, ATK: 400, DEF: 300, MANA: 6)")
	fmt.Println("  Queen  (Special: Heal, MANA: 5)")
	if state.EndTime != "" {
		end, _ := time.Parse(time.RFC3339, state.EndTime) // Parse end time
		now := time.Now()
		remain := end.Sub(now) // Calculate time left
		if remain > 0 {
			fmt.Printf("Time left: %v\n", remain.Truncate(time.Second)) // Print time left
		}
	}
	fmt.Println("=========================================")
	fmt.Println("[ENHANCED MODE] Type: buy <troop> | deploy <troop> <tower> | exit")
}

// enhancedInputLoop handles user input for enhanced mode in a separate goroutine
// Allows the user to buy or deploy troops, or exit the game, while the game state updates in real time
func enhancedInputLoop(conn net.Conn) {
	reader := bufio.NewReader(os.Stdin) // Reader for user input

	isRunning := true               // Flag to control the loop
	stopCh := enhancedInputStop     // Channel to signal when to stop
	inputCh := make(chan string, 5) // Buffered channel for user input

	// Goroutine to read user input and send to inputCh
	go func() {
		for isRunning {
			select {
			case <-stopCh:
				isRunning = false // Stop if signaled
				return
			default:
				fmt.Print("[ENHANCED] > ")           // Prompt
				line, err := reader.ReadString('\n') // Read input
				if err != nil {
					isRunning = false // Stop on error
					return
				}
				trimmedLine := strings.TrimSpace(line) // Remove whitespace
				// Check again for stop signal before sending input
				select {
				case <-stopCh:
					isRunning = false
					return
				default:
					if isRunning && trimmedLine != "" {
						inputCh <- trimmedLine // Send input to channel
					}
				}
			}
		}
	}()

	// Main loop to process user input from inputCh
	for isRunning {
		select {
		case <-stopCh:
			isRunning = false
			fmt.Println("[Double enter to return to menu]")
			return
		case line := <-inputCh:
			if !isRunning {
				return
			}
			if line == "exit" {
				conn.Write([]byte("EXIT_GAME\n")) // Send exit command to server
				fmt.Println("Returning to main menu...")
				isRunning = false
				return
			}
			if strings.HasPrefix(line, "deploy ") {
				parts := strings.Fields(line)
				if len(parts) == 3 {
					cmd := fmt.Sprintf("DEPLOY|%s|%s\n", parts[1], parts[2])
					conn.Write([]byte(cmd)) // Send deploy command
					fmt.Println("[Sent deploy command]")
				} else {
					fmt.Println("Usage: deploy <troop> <tower>")
				}
			} else if strings.HasPrefix(line, "buy ") {
				parts := strings.Fields(line)
				if len(parts) == 2 {
					cmd := fmt.Sprintf("BUY|%s\n", parts[1])
					conn.Write([]byte(cmd)) // Send buy command
					fmt.Println("[Sent buy command]")
				} else {
					fmt.Println("Usage: buy <troop>")
				}
			} else {
				fmt.Println("Unknown command. Use: buy <troop> | deploy <troop> <tower> | exit")
			}
		}
	}
}

// inGameLoop handles user input for simple mode (turn-based)
// scanner: reads server messages
// conn: network connection
// mode: game mode (simple/enhanced)
// myTurn: whether it's the player's turn
// cachedState: last known game state string
func inGameLoop(scanner *bufio.Scanner, conn net.Conn, mode string, myTurn bool, cachedState string) {
	reader := bufio.NewReader(os.Stdin) // Reader for user input

	// Always display current game state when entering the game loop
	if cachedState != "" {
		fmt.Println("[Trạng thái game hiện tại]")
		fmt.Println(cachedState)
	}

	// Only show menu if it's the player's turn
	if myTurn {
		fmt.Print("Enter command: 1. Deploy  3. Exit Game\n> ")
		cmd, _ := reader.ReadString('\n')
		cmd = strings.TrimSpace(cmd)
		if cmd == "1" {
			fmt.Print("Troop name: ")
			troop, _ := reader.ReadString('\n')
			troop = strings.TrimSpace(troop)
			fmt.Print("Target tower: ")
			tower, _ := reader.ReadString('\n')
			tower = strings.TrimSpace(tower)
			// Send deploy command to server
			conn.Write([]byte("DEPLOY|" + troop + "|" + tower + "\n"))
			fmt.Println("[Sending deploy command...]")
			return // Return to listening for server messages
		} else if cmd == "3" {
			fmt.Println("Exiting game...")
			conn.Write([]byte("EXIT_GAME\n"))
			fmt.Println("Returning to main menu...")
			return
		} else if cmd != "" {
			fmt.Println("Lệnh không hợp lệ, vui lòng thử lại.")
			return // Return to turn loop for another try
		}
	} else {
		// Nếu chờ đối thủ, chỉ cho phép người chơi thoát game
		fmt.Print("Enter 'exit' to quit game\n> ")
		cmd, _ := reader.ReadString('\n')
		cmd = strings.ToLower(strings.TrimSpace(cmd))
		if cmd == "exit" || cmd == "3" {
			fmt.Println("Exiting game...")
			conn.Write([]byte("EXIT_GAME\n"))
			fmt.Println("Returning to main menu...")
			return
		}
		// Không thực hiện thêm hành động nào, trở về chế độ lắng nghe
		return
	}
}
