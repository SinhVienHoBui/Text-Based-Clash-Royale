# Text-Based Clash Royale (TCR)

# INTRODUCTION

## Abstract
As a term project for the Net-Centric Programming course, the Text-Based Clash Royale (TCR) project is a simplified, command-line multiplayer strategy game developed in Go, inspired by the popular mobile game Clash Royale by Supercell. TCR demonstrates the use of TCP (and optionally UDP) protocols for network communication. The game features turn-based or real-time gameplay, strategic troop deployment, and persistent player progress through experience and leveling systems. This project reinforces networking concepts and challenges students to design, prototype, and test a multiplayer networked application.

## Key Features
- **Networked Multiplayer:** Two players connect to a server using TCP to engage in 1v1 combat.
- **Tower Defense Mechanics:** Each player has 3 towers (1 King Tower, 2 Guard Towers) to defend and attack.
- **Troop Deployment:** Players summon troops with different stats and abilities using MANA.
- **Game Modes:**
  - *Simple Mode:* Turn-based with basic attack-defense interactions.
  - *Enhanced Mode:* Real-time combat, CRIT chance, EXP-based leveling, and MANA management.
- **EXP & Leveling System:** Players earn experience and level up, boosting their stats.
- **JSON-Based Persistence:** All player and troop/tower data is stored and managed via JSON files.
- **MANA System:** Governs troop summoning with timed regeneration and strategic usage.

## Achievements
- Implemented logical game rules and mechanics.
- Built a functional multiplayer game using TCP sockets in Golang.
- Established successful connections between server and clients.
- Experienced full-cycle software development from design to demonstration.
- Managed source code and collaboration using GitHub.
- Applied theoretical networking knowledge in a practical context.
- Achieved a playable and extendable base for future game development.

## Techniques and Tools
- **Golang Programming Language:** Utilized for its strong concurrency and networking capabilities.
- **Game Architecture:** Client-server model; server handles logic, client sends commands and receives responses.
- **Version Control:** GitHub for source code management and team coordination.
- **Design Tool:** Canva for creating diagrams and visual assets.

# GAME RULES & FEATURES

## The Rule of the Game
- Each player controls 3 towers: 1 King Tower and 2 Guard Towers.
- Players take turns (Simple Mode) or act in real-time (Enhanced Mode) to deploy troops and attack opponent towers.
- Troops have unique stats (HP, ATK, DEF, MANA cost) and some have special abilities (e.g., Queen can heal towers).
- Players must destroy at least one Guard Tower before attacking the King Tower.
- The game ends when a King Tower is destroyed or time runs out (Enhanced Mode).
- Players earn EXP and level up after each match, improving their stats.

## Game Features
- Turn-based and real-time gameplay modes.
- Strategic troop deployment and MANA management.
- Persistent player progress and leveling.
- JSON-based data storage for users and game specs.
- Text-based interface for easy access and testing.

# GAME DETAILS

## Initialization and Connection
- The server is started and listens for incoming TCP connections.
- Clients connect to the server and are prompted to login or register.

## User Registration
- New users can register with a username and password.
- User data is stored in `data/users.json`.

## Clash Royale Selection
- Players can create or join game rooms.
- Game rooms can be set to SIMPLE or ENHANCED mode.

## Main Interaction Loop
- Players send commands to the server (e.g., DEPLOY, EXIT_GAME).
- The server processes commands, updates game state, and sends responses.
- Game state and results are communicated via defined PDUs.

## Receiving Messages
- Clients receive and display game state updates, attack results, turn notifications, and end-of-game messages.
- All communication follows the PDU protocol described in the documentation.

# FUTURE DEVELOPMENT
- Add support for more than two players or team-based modes.
- Implement a graphical user interface (GUI).
- Integrate UDP for real-time features and performance testing.
- Expand troop and tower types with new abilities.
- Add matchmaking and ranking systems.
- Improve security (e.g., password hashing, input validation).

# CONCLUSION
The Text-Based Clash Royale project successfully demonstrates the application of networking concepts in a practical, multiplayer game. It provides a solid foundation for further development and experimentation with networked applications, game mechanics, and persistent data management.

# REFERENCES
- [Go Programming Language](https://golang.org/)
- [Clash Royale by Supercell](https://clashroyale.com/)
- [GitHub](https://github.com/)
- [Canva](https://www.canva.com/)
- Course materials and lectures on Net-Centric Programming

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
