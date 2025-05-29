# Text-Based Clash Royale (TCR)

## System Architecture
- **Server:** Implemented in Go, manages authentication, matchmaking, and all game logic. Maintains game state, processes client commands, and sends updates.
- **Client:** Each client connects to the server via TCP, sends user commands (login, deploy troop, etc.), and displays game state updates.
- **Data Storage:** All persistent data (user accounts, troop/tower specs, player progress) is stored in JSON files in the `data/` directory.

## Application PDU Description

This section outlines the PDUs for client-server communication.

### Client to Server:
- **`LOGIN|user|pass`**: Log in.
- **`REGISTER|user|pass`**: Create account.
- **`CREATE_GAME|MODE`**: New game (`SIMPLE` or `ENHANCED`).
- **`LIST_GAMES`**: Request available game rooms.
- **`JOIN_GAME|id`**: Join game room by ID.
- **`DEPLOY|troop|target`**: Deploy troop to tower.
- **`BUY|troop`**: (Enhanced) Purchase troop.
- **`EXIT_GAME`**: (Optional) Leave game/disconnect.

### Server to Client:
- **`ACK|LOGIN_SUCCESS`**: Login successful.
- **`ERR|LOGIN_FAILED|reason`**: Login failed.
- **`ACK|REGISTER_SUCCESS`**: Registration successful.
- **`ERR|REGISTER_FAILED|reason`**: Registration failed.
- **`ACK|GAME_CREATED|id`**: Game room created.
- **`ERR|CREATE_GAME_FAILED|reason`**: Game creation failed.
- **`GAMES|id1:host1,...`**: List of games or `No available rooms`.
- **`ACK|JOINED|id`**: Joined game room.
- **`ERR|JOIN_FAILED|reason`**: Join game failed.
- **`ACK|GAME_STARTED`**: Game ready to start.
- **`GAME_START|p1_user|p2_user`**: Match start, identifies players.
- **`STATE|<json_state>`**: Current game state (JSON).
- **`ATTACK_RESULT|atk_id|def_id|dmg|def_hp`**: Attack outcome.
- **`QUEEN_HEAL|player|tower|heal|new_hp`**: (Enhanced) Queen heal action.
- **`ACK|BUY_SUCCESS`**: (Enhanced) Troop purchase successful.
- **`ERR|BUY_FAILED|reason`**: (Enhanced) Troop purchase failed.
- **`ACK|DEPLOY_SUCCESS`**: Troop deployment successful.
- **`ERR|DEPLOY_FAILED|reason`**: Troop deployment failed.
- **`TURN|Your turn!` / `Wait...`**: (Simple) Turn indicator.
- **`GAME_END|winner_status|reason`**: Game end, winner, reason.
- **`ERR|message`**: Generic error.

## Sequence Diagram

Client1         Server         Client2
   |              |              |
   |--LOGIN------>|              |
   |<-ACK/ERR-----|              |
   |              |--LOGIN------>|
   |              |<-ACK/ERR-----|
   |--CREATE_GAME>|              |
   |<-ACK---------|              |
   |              |<--LIST_GAMES-|
   |              |--GAMES------>|
   |              |<--JOIN_GAME--|
   |              |--ACK-------->|
   |<-GAME_START--|--GAME_START->|
   |<-STATE-------|--STATE------>|
   |--DEPLOY----->|              |
   |<-ACK/RESULT--|--STATE------>|
   |              |<--DEPLOY-----|
   |--STATE------>|<-ACK/RESULT--|
   |<-GAME_END----|--GAME_END--->|
```

## Deployment & Execution Instructions
1. **Install Go:** [https://golang.org/dl/](https://golang.org/dl/)
2. **Clone the repository:**
   ```powershell
   git clone <repo-url>
   cd Text-Based-Clash-Royale
   ```
3. **Prepare Data:** Place JSON data files in the `data/` directory.
4. **Run the server:**
   ```powershell
   go run server/server.go
   ```
5. **Run the client (in another terminal):**
   ```powershell
   go run client/client.go
   ```
6. **Follow on-screen instructions to play.**
