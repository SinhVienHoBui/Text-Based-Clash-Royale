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

func main() {
	fmt.Println("TCR Client starting...")
	conn, err := net.Dial("tcp", "localhost:9000")
	if err != nil {
		fmt.Println("Unable to connect to server:", err)
		os.Exit(1)
	}
	defer conn.Close()
	fmt.Println("Connected to server.")
	reader := bufio.NewReader(os.Stdin)
	serverScanner := bufio.NewScanner(conn)
	// Login/Register
	for {
		fmt.Println("1. Login\n2. Register\nChoose:")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)
		if choice == "1" {
			fmt.Print("Username: ")
			user, _ := reader.ReadString('\n')
			fmt.Print("Password: ")
			pass, _ := reader.ReadString('\n')
			msg := fmt.Sprintf("LOGIN|%s|%s", strings.TrimSpace(user), strings.TrimSpace(pass))
			conn.Write([]byte(msg + "\n"))
		} else if choice == "2" {
			fmt.Print("Username: ")
			user, _ := reader.ReadString('\n')
			fmt.Print("Password: ")
			pass, _ := reader.ReadString('\n')
			msg := fmt.Sprintf("REGISTER|%s|%s", strings.TrimSpace(user), strings.TrimSpace(pass))
			conn.Write([]byte(msg + "\n"))
		} else {
			fmt.Println("Invalid choice.")
			continue
		}
		// Wait for ACK
		for serverScanner.Scan() {
			msg := serverScanner.Text()
			fmt.Println(msg)
			if strings.HasPrefix(msg, "ACK|Login successful") || strings.HasPrefix(msg, "ACK|Register successful") {
				break
			} else if strings.HasPrefix(msg, "ERR|") {
				os.Exit(1)
			}
		}
		break
	} // end login/register	// After login/register, allow game creation/joining
	for {
		fmt.Println("1. Create Game\n2. List/Join Game\n3. Exit\nChoose:")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)
		if choice == "3" {
			fmt.Println("Exiting client. Goodbye!")
			os.Exit(0)
		} else if choice == "1" {
			fmt.Println("Select game mode: 1. Simple  2. Enhanced")
			mode, _ := reader.ReadString('\n')
			mode = strings.TrimSpace(mode)
			if mode == "2" {
				conn.Write([]byte("CREATE_GAME|ENHANCED\n"))
			} else {
				conn.Write([]byte("CREATE_GAME|SIMPLE\n"))
			}
			waitForGameStart(serverScanner, conn, mode)
		} else if choice == "2" {
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
			fmt.Print("Enter room id to join: ")
			room, _ := reader.ReadString('\n')
			room = strings.TrimSpace(room)
			if room != "" {
				conn.Write([]byte("JOIN_GAME|" + room + "\n"))
				waitForGameStart(serverScanner, conn, "")
			}
		} else {
			fmt.Println("Invalid choice.")
		}
	}
}

// Only one scanner reads from server, and all game logic is handled here
func waitForGameStart(scanner *bufio.Scanner, conn net.Conn, mode string) {
	staticEnhancedInputOnce = sync.Once{}   // Reset for every new game
	enhancedInputStop = make(chan struct{}) // Reset stop channel for each game
	fmt.Println("[Waiting for game to start...]")
	for scanner.Scan() {
		msg := scanner.Text()
		fmt.Println(msg)
		if strings.HasPrefix(msg, "ACK|GAME_STARTED") {
			fmt.Println("[Game started successfully!]")
			if mode != "2" && mode != "ENHANCED" {
				fmt.Println("[Waiting for turn info...]")
			}
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
	myTurn := false
	currentState := ""
	waitingForInput := false
	isEnhanced := (mode == "2" || mode == "ENHANCED")
	// Nếu mode rỗng (join phòng), tự động nhận diện ENHANCED nếu nhận được STATE|{...json...}
	var enhancedDetected bool
	if isEnhanced {
		staticEnhancedInputOnce.Do(func() {
			go enhancedInputLoop(conn)
		})
	}
	for scanner.Scan() {
		msg := scanner.Text()

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
					// Only close if not already closed
					select {
					case <-enhancedInputStop:
						// already closed, do nothing
					default:
						close(enhancedInputStop)
						time.Sleep(300 * time.Millisecond)
					}
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

func printEnhancedState(state *EnhancedGameState, conn net.Conn) {
	fmt.Println("\n========== ENHANCED GAME STATE ==========")
	fmt.Printf("Room: %s\n", state.RoomID)
	for uname, p := range state.Players {
		fmt.Printf("Player: %s (Level %d, EXP %d, Mana %d)\n", uname, p.Level, p.EXP, p.Mana)
		fmt.Println("  Towers:")
		for _, t := range []string{"Guard1", "Guard2", "King"} {
			tower := p.Towers[t]
			// Show CRIT for tower
			crit := ""
			if t == "King" {
				crit = "CRIT: 10%"
			} else {
				crit = "CRIT: 5%"
			}
			fmt.Printf("    %s: HP=%d ATK=%d DEF=%d %s\n", tower.Name, tower.HP, tower.ATK, tower.DEF, crit)
		}
		fmt.Println("  Troops:")
		for _, tr := range p.Troops {
			mana := 0
			crit := ""
			switch tr.Name {
			case "Pawn":
				mana = 3
			case "Bishop":
				mana = 4
			case "Rook":
				mana = 5
			case "Knight":
				mana = 5
			case "Prince":
				mana = 6
			case "Queen":
				mana = 5
			}
			fmt.Printf("    %s: HP=%d ATK=%d DEF=%d MANA=%d %s\n", tr.Name, tr.HP, tr.ATK, tr.DEF, mana, crit)
		}
	}
	if state.EndTime != "" {
		end, _ := time.Parse(time.RFC3339, state.EndTime)
		now := time.Now()
		remain := end.Sub(now)
		if remain > 0 {
			fmt.Printf("Time left: %v\n", remain.Truncate(time.Second))
		}
	}
	fmt.Println("=========================================")
	fmt.Println("[ENHANCED MODE] Type: deploy <troop> <tower> | exit")
}

// Completely rewritten to fix issues with input handling and game ending
func enhancedInputLoop(conn net.Conn) {
	reader := bufio.NewReader(os.Stdin)

	// Sử dụng một biến cờ để theo dõi trạng thái hoạt động
	isRunning := true

	// Sử dụng một tham chiếu cục bộ để tránh sử dụng biến toàn cục
	stopCh := enhancedInputStop

	// Tạo một channel riêng để xử lý input
	inputCh := make(chan string, 5) // Buffer lớn hơn để tránh blocking

	// Goroutine đọc input từ người dùng
	go func() {
		for isRunning {
			select {
			case <-stopCh:
				isRunning = false
				return
			default:
				fmt.Print("[ENHANCED] > ")
				line, err := reader.ReadString('\n')
				if err != nil {
					isRunning = false
					return
				}

				trimmedLine := strings.TrimSpace(line)

				// Kiểm tra lại nếu đã nhận tín hiệu dừng trong khi đọc
				select {
				case <-stopCh:
					isRunning = false
					return
				default:
					if isRunning && trimmedLine != "" {
						// Gửi input vào channel
						inputCh <- trimmedLine
					}
				}
			}
		}
	}()

	// Vòng lặp chính xử lý input
	for isRunning {
		select {
		case <-stopCh:
			isRunning = false
			fmt.Println("[Enhanced mode stopped]")
			return

		case line := <-inputCh:
			if !isRunning {
				return
			}

			if line == "exit" {
				conn.Write([]byte("EXIT_GAME\n"))
				fmt.Println("Returning to main menu...")
				isRunning = false
				return
			}

			if strings.HasPrefix(line, "deploy ") {
				parts := strings.Fields(line)
				if len(parts) == 3 {
					cmd := fmt.Sprintf("DEPLOY|%s|%s\n", parts[1], parts[2])
					conn.Write([]byte(cmd))
					fmt.Println("[Sent deploy command]")
				} else {
					fmt.Println("Usage: deploy <troop> <tower>")
				}
			} else {
				fmt.Println("Unknown command. Use: deploy <troop> <tower> or exit")
			}
		}
	}
}

// Refactored: always allow STATE and EXIT, only allow DEPLOY on your turn
func inGameLoop(scanner *bufio.Scanner, conn net.Conn, mode string, myTurn bool, cachedState string) {
	reader := bufio.NewReader(os.Stdin)

	// Always display current game state when entering the game loop
	if cachedState != "" {
		fmt.Println("[Trạng thái game hiện tại]")
		fmt.Println(cachedState)
	}

	// Chỉ hiển thị menu cho người đang có lượt
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

			// Clear any input buffer before sending command
			conn.Write([]byte("DEPLOY|" + troop + "|" + tower + "\n"))
			fmt.Println("[Sending deploy command...]")
			// Return to listening for server messages
			return
		} else if cmd == "3" {
			fmt.Println("Exiting game...")
			conn.Write([]byte("EXIT_GAME\n"))
			fmt.Println("Returning to main menu...")
			return
		} else if cmd != "" {
			fmt.Println("Lệnh không hợp lệ, vui lòng thử lại.")
			// Return to the turn loop to receive proper instructions
			return
		}
	} else {
		// Khi chờ đối thủ, chỉ cho phép người chơi thoát game
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
