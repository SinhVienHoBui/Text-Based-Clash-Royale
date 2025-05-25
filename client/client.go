package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
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
	} // end login/register
	// After login/register, allow game creation/joining
	for {
		fmt.Println("1. Create Game\n2. List/Join Game\nChoose:")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)
		if choice == "1" {
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
	fmt.Println("[Waiting for game to start...]")
	for scanner.Scan() {
		msg := scanner.Text()
		fmt.Println(msg)
		if strings.HasPrefix(msg, "ACK|GAME_STARTED") {
			fmt.Println("[Game started successfully!]")
			fmt.Println("[Waiting for turn info...]")
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

	for scanner.Scan() {
		msg := scanner.Text()

		// Process different message types
		if strings.HasPrefix(msg, "STATE|") {
			fmt.Println("[Game State Update]")
			currentState = msg[6:]
			fmt.Println(currentState)
		} else if strings.HasPrefix(msg, "ATTACK_RESULT|") {
			fmt.Println("[Attack Result]")
			fmt.Println(msg)
		} else if strings.HasPrefix(msg, "GAME_END|") {
			fmt.Println("[Game End]")
			fmt.Println(msg)
			os.Exit(0)
		} else if strings.HasPrefix(msg, "TURN|Your turn!") {
			myTurn = true
			fmt.Println("[Turn Update] Your turn!")

			// Hiển thị menu khi đến lượt người chơi
			if !waitingForInput {
				waitingForInput = true
				go func() {
					inGameLoop(scanner, conn, mode, myTurn, currentState)
					waitingForInput = false
				}()
			}
		} else if strings.HasPrefix(msg, "TURN|Wait for your turn...") {
			myTurn = false
			fmt.Println("[Turn Update] Wait for your turn...")
			fmt.Println("[Đối thủ đã thực hiện xong lượt đi]")
			fmt.Println("[Chờ đến lượt của bạn...]")
		} else if strings.HasPrefix(msg, "ERR|") {
			// Handle error messages
			fmt.Println("[Error]", msg[4:])
		} else if strings.HasPrefix(msg, "ACK|") {
			// Chỉ hiển thị ACK khi đó thực sự là xác nhận của lệnh mình đã gửi
			if myTurn {
				fmt.Println("[Server]", msg[4:])
			}
		} else {
			// Handle other server messages
			fmt.Println(msg)
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
		fmt.Print("Enter command: 1. Deploy  3. Exit\n> ")

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
			os.Exit(0)
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
			os.Exit(0)
		}

		// Không thực hiện thêm hành động nào, trở về chế độ lắng nghe
		return
	}
}
