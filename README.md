# Text-Based Clash Royale (TCR)

## System Architecture
- **Server**: Manages player authentication, game room creation/joining, match logic, progress saving, and communicates with multiple clients via TCP. Supports two modes: SIMPLE (basic) and ENHANCED (advanced: mana, exp, level, timer).
- **Client**: Connects to the server via TCP, sends commands (login, create/join room, deploy troop, etc.), receives and displays game state, supports both game modes.
- **Data Storage**: User data, troop/tower specs, and player progress are stored in JSON files in the `data/` directory.

## Application PDU Description
Messages (PDUs) exchanged between client and server:
- **LOGIN|username|password**: Login.
- **REGISTER|username|password**: Register a new account.
- **CREATE_GAME|SIMPLE|ENHANCED**: Create a game room (simple/advanced mode).
- **LIST_GAMES**: List available waiting rooms.
- **JOIN_GAME|room_id**: Join a room.
- **START_GAME|room_id**: Start the match (SIMPLE mode).
- **DEPLOY|troop_name|target_tower**: Deploy a troop to attack an opponent's tower.
- **STATE|<json_state>**: Server sends the current game state (text or JSON depending on mode).
- **ATTACK_RESULT|attacker|defender|damage|remaining_hp|CRIT:true/false|DESTROYED**: Attack result.
- **QUEEN_HEAL|player|tower|heal_amount|new_hp**: Queen heals a tower.
- **TURN|Your turn! / Wait for your turn...**: Turn notification.
- **GAME_END|result**: End of match (win/lose/draw/reason).
- **ACK|...**: Success notification.
- **ERR|...**: Error notification.
- **EXIT_GAME**: Leave the match and return to menu.

## Sequence Diagram
```
Client1      Server      Client2
   |           |           |
   |--LOGIN--->|           |
   |<--ACK-----|           |
   |--CREATE-->|           |
   |<--ACK-----|           |
   |           |<--LOGIN-- |
   |           |---ACK---> |
   |           |<--LIST--  |
   |           |---GAMES-> |
   |           |<--JOIN--- |
   |           |---ACK---> |
   |<--GAME START/STATE--->|
   |---DEPLOY-->|           |
   |<--ATTACK_RESULT/STATE/QUEEN_HEAL--->|
   |           |           |
   |<--GAME_END/STATE----->|
```

## Deployment & Execution Instructions
1. Install Go (https://golang.org/dl/)
2. Clone the repository.
3. Make sure the JSON data files exist in the `data/` directory (`users.json`, `specs.json`).
4. Run the server:
   ```powershell
   go run server/server.go
   ```
5. Run the client (each player opens a separate terminal):
   ```powershell
   go run client/client.go
   ```
6. Login/register, create or join a room, and follow the on-screen instructions to play.

### Notes
- ENHANCED mode adds mana, exp, level, timer, Queen healing, etc.
- Troop/tower stats can be edited in `data/specs.json`.
- Player progress is saved in `data/users.json`.
