# Text-Based Clash Royale (TCR)

## System Architecture
- **Server**: Handles player authentication, matchmaking, and game logic. Manages game state and communication between two clients.
- **Client**: Connects to the server, sends user commands, and displays game state updates.
- **Data Storage**: JSON files for user accounts, troop/tower specs, and player progress.

## Application PDU Description
- **Login**: `LOGIN|username|password`
- **Game Start**: `GAME_START|player1|player2`
- **Deploy Troop**: `DEPLOY|troop_name|target_tower`
- **Attack Result**: `ATTACK_RESULT|attacker|defender|damage|remaining_hp`
- **Game State Update**: `STATE|<json_state>`
- **Game End**: `GAME_END|winner|reason`

## Sequence Diagram
```
Client1      Server      Client2
   |           |           |
   |--Login--->|           |
   |<--Ack-----|           |
   |           |<--Login-- |
   |           |---Ack---> |
   |<--Game Start Info---->|
   |---Deploy Troop------->|
   |<--Attack Result------>|
   |           |           |
   |<--Game End Info------>|
```

## Deployment & Execution Instructions
1. Install Go (https://golang.org/dl/)
2. Clone the repository.
3. Place JSON data files in the `data/` directory.
4. Run the server:
   ```sh
   go run server/server.go
   ```
5. Run the client (in another terminal):
   ```sh
   go run client/client.go
   ```
6. Follow on-screen instructions to play.

---

*See Appendix in the project prompt for troop/tower stats.*
