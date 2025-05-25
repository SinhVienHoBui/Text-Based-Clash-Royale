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
	for scanner.Scan() {
		msg := scanner.Text()
		fmt.Println(msg)
		if strings.HasPrefix(msg, "ACK|GAME_STARTED") {
			listenTurnLoop(scanner, conn, mode)
			return
		}
		if strings.HasPrefix(msg, "ERR|") {
			os.Exit(1)
		}
	}
}

func listenTurnLoop(scanner *bufio.Scanner, conn net.Conn, mode string) {
	myTurn := false
	for scanner.Scan() {
		msg := scanner.Text()
		if strings.HasPrefix(msg, "STATE|") {
			fmt.Println("[Game State Update]")
			fmt.Println(msg[6:])
			// After showing state, continue waiting for TURN message (do not return to menu)
			continue
		} else if strings.HasPrefix(msg, "ATTACK_RESULT|") {
			fmt.Println("[Attack Result]")
			fmt.Println(msg)
			continue
		} else if strings.HasPrefix(msg, "GAME_END|") {
			fmt.Println("[Game End]")
			fmt.Println(msg)
			os.Exit(0)
		} else if strings.HasPrefix(msg, "TURN|Your turn!") {
			myTurn = true
			inGameLoop(scanner, conn, mode, myTurn)
			return
		} else if strings.HasPrefix(msg, "TURN|Wait for your turn...") {
			myTurn = false
			inGameLoop(scanner, conn, mode, myTurn)
			return
		} else {
			fmt.Println(msg)
		}
	}
}

// Refactored: always allow STATE and EXIT, only allow DEPLOY on your turn
func inGameLoop(scanner *bufio.Scanner, conn net.Conn, mode string, myTurn bool) {
	reader := bufio.NewReader(os.Stdin)
	for {
		if myTurn {
			fmt.Print("Enter command: 1. Deploy  2. State  3. Exit\n> ")
		} else {
			fmt.Println("Chờ đối thủ... (bạn chỉ có thể xem trạng thái hoặc thoát)")
			fmt.Print("Enter command: 2. State  3. Exit\n> ")
		}
		cmd, _ := reader.ReadString('\n')
		cmd = strings.TrimSpace(cmd)
		if myTurn && cmd == "1" {
			fmt.Print("Troop name: ")
			troop, _ := reader.ReadString('\n')
			troop = strings.TrimSpace(troop)
			fmt.Print("Target tower: ")
			tower, _ := reader.ReadString('\n')
			tower = strings.TrimSpace(tower)
			conn.Write([]byte("DEPLOY|" + troop + "|" + tower + "\n"))
			// After deploy, immediately return to listenTurnLoop to wait for next TURN
			return
		} else if cmd == "2" {
			conn.Write([]byte("STATE\n"))
			// Wait for STATE| response and print it, then continue in-game menu
			for scanner.Scan() {
				msg := scanner.Text()
				if strings.HasPrefix(msg, "STATE|") {
					fmt.Println("[Game State Update]")
					fmt.Println(msg[6:])
					// After showing state, continue waiting for TURN message (do not return to menu)
					break
				} else if strings.HasPrefix(msg, "ERR|") {
					fmt.Println(msg)
					break
				} else if strings.HasPrefix(msg, "GAME_END|") {
					fmt.Println("[Game End]")
					fmt.Println(msg)
					os.Exit(0)
				} else if strings.HasPrefix(msg, "TURN|Your turn!") {
					myTurn = true
					break
				} else if strings.HasPrefix(msg, "TURN|Wait for your turn...") {
					myTurn = false
					break
				} else {
					fmt.Println(msg)
				}
			}
			// After showing state, continue the in-game loop (do not return)
			continue
		} else if cmd == "3" {
			fmt.Println("Exiting game...")
			os.Exit(0)
		} else {
			fmt.Println("Invalid command.")
		}
	}
}
