# Text-Based Clash Royale (TCR) - Report

## INTRODUCTION
### Abstract
As a term project for the Net-Centric Programming course, the Text-Based Clash Royale (TCR) project is a simplified, command-line multiplayer strategy game developed in Go, inspired by Clash Royale by Supercell. TCR demonstrates the use of TCP protocols for network communication, featuring turn-based or real-time gameplay, strategic troop deployment, and persistent player progress. The project reinforces networking concepts and challenges students to design, prototype, and test a multiplayer networked application.

### Key Features
- **Networked Multiplayer:** Two players connect to a server using TCP for 1v1 combat.
- **Tower Defense Mechanics:** Each player has 3 towers (1 King Tower, 2 Guard Towers) to defend and attack.
- **Troop Deployment:** Players summon troops with unique stats and abilities using MANA.
- **Game Modes:**
  - Simple Mode: Turn-based, basic attack-defense.
  - Enhanced Mode: Real-time combat, CRIT chance, EXP-based leveling.
- **EXP & Leveling System:** Players earn experience and level up, boosting stats.
- **JSON-Based Persistence:** All player and troop/tower data stored in JSON files.
- **MANA System:** Governs troop summoning with timed regeneration and strategic usage.

### Achievement
- Implemented logical game rules and a functional multiplayer game using TCP sockets in Go.
- Established successful server-client connections.
- Experienced full-cycle software development from design to demonstration.
- Managed source code and teamwork via GitHub.
- Applied networking protocol theory in practice.
- Built a playable and extendable base for future development.

### Techniques And Tools
- **Golang:** Used for both server and client for concurrency and networking.
- **Game Architecture:** Client-server model; server handles logic, client sends commands and receives responses.
- **Version Control:** GitHub for code management and collaboration.
- **Design Tool:** Canva for diagrams and visual assets.

## GAME RULES & FEATURES
### The Rule of the Game
- **Simple TCR Rules:**
    - **Turn-Based Combat:** Players take turns to perform actions. One player completes their action (e.g., deploying a troop) before the other player can act.
    - **Basic Attack-Defense:** Troops attack designated enemy towers. Towers automatically counter-attack the troop that attacked them.
    - **Objective:** Destroy the opponent's King Tower or have more standing towers/higher total tower HP when a turn limit or other end condition is met.
    - **Troop Deployment:** Players deploy troops from a predefined set. There is no MANA or resource cost in the simple mode.
    - **Winning Condition:** The first player to destroy the opponent's King Tower wins. If no King Tower is destroyed after a certain number of turns or a time limit, the player with more remaining tower HP or more standing towers might be declared the winner (specifics depend on exact server implementation).
- **Enhanced TCR Rules:**
    - **Real-Time Combat (Simulated):** While the core interaction might still be command-based, the game aims for a more dynamic feel. Events like MANA regeneration and troop actions occur continuously or based on timers, rather than strictly waiting for player turns for all actions.
    - **MANA System:** Players have a MANA pool that regenerates over time. Deploying troops costs MANA, requiring strategic resource management. Different troops have different MANA costs as defined in `data/specs.json`.
    - **CRIT Chance:** Troops and towers have a chance to inflict critical hits for increased damage, adding an element of randomness and excitement. CRIT chances are defined in `data/specs.json`.
    - **EXP & Leveling System:** Players gain Experience Points (EXP) for actions like destroying enemy troops or towers. Gaining enough EXP allows players to level up. Leveling up can increase player stats, tower strength, or troop effectiveness. Player progress (EXP, Level) is stored in `data/users.json`.
    - **Special Troop Abilities:** Some troops may have special abilities, like the "Queen" troop's "heal" ability mentioned in `data/specs.json`.
    - **Winning Condition:** Similar to Simple Mode (destroy King Tower), but potentially with more complex scenarios based on real-time events and resource management.

### Game Features
- **Simple TCR Features:**
    - Turn-based gameplay.
    - Direct troop vs. tower combat.
    - Predefined troop and tower stats (though `specs.json` is used, the "simple" mode might ignore MANA, CRIT, Special fields).
    - Basic win/loss conditions.
- **Enhanced TCR Features:**
    - **MANA Management:** Players must strategically manage their MANA to deploy troops. MANA regenerates over time.
    - **EXP and Leveling:** Players earn EXP from battles, level up, and potentially improve their stats or unlock stronger units/abilities. This progress is persistent.
    - **Critical Hits (CRIT):** Adds a chance factor to combat, making battles less predictable.
    - **Diverse Troop Roster:** A variety of troops with different stats (HP, ATK, DEF), MANA costs, and potential special abilities (e.g., Pawn, Bishop, Rook, Knight, Prince, Queen from `data/specs.json`).
    - **Tower Specialization:** Towers (King, Guard1, Guard2) have distinct stats and importance.
    - **JSON-Defined Specifications:** Troop and tower characteristics (HP, ATK, DEF, MANA, CRIT, EXP rewards, special abilities) are loaded from `data/specs.json`, allowing for easier balancing and expansion.
    - **Persistent Player Data:** User accounts, including EXP and level, are stored in `data/users.json`.

## GAME DETAILS
- **Initialization and Connection:**
    - The client (`client/client.go`) initiates a TCP connection to the server (`server/server.go`) at a predefined address and port (e.g., `localhost:9000`).
    - Upon successful connection, the server acknowledges, and the client is typically presented with options to log in or register.
- **User Registration:**
    - New users can choose to register. The client sends a registration request (e.g., `REGISTER|username|password`) to the server.
    - The server (`server/server.go`) checks if the username already exists. If not, it stores the new user's credentials (username and hashed password, along with initial EXP/Level) in the `data/users.json` file.
    - The server sends an acknowledgment (`ACK|REGISTER_SUCCESS` or `ERR|USERNAME_EXISTS`) back to the client.
- **User Login:**
    - Existing users can log in by sending their credentials (e.g., `LOGIN|username|password`).
    - The server validates these credentials against the data stored in `data/users.json`.
    - A successful login is acknowledged (`ACK|LOGIN_SUCCESS`), and an error is sent for failures (`ERR|LOGIN_FAILED`).
- **Game Creation and Joining (Lobby System):**
    - After login, players can typically create a new game room or list and join existing ones.
    - **Create Game:** A player can send a command like `CREATE_GAME|mode` (where mode is "SIMPLE" or "ENHANCED"). The server creates a new game room, assigns it an ID, and sets the creating player as the host.
    - **List/Join Game:** A player can request a list of available game rooms. To join, they send a command like `JOIN_GAME|room_id`.
    - The server manages game rooms, tracking hosts, guests, and game status (waiting, started).
- **Game Start:**
    - Once two players are in a game room (host and guest), the game can start. The server sends a `GAME_START|player1|player2` message to both clients.
    - For Simple TCR, the server might randomly decide who takes the first turn.
    - For Enhanced TCR, the server initializes player-specific states like MANA, loads troop/tower specs based on player levels if applicable, and starts any game timers (e.g., for MANA regeneration).
- **Clash Royale Selection (Troop Deployment & Actions):**
    - **Simple Mode:** On their turn, a player sends a command like `DEPLOY|troop_name|target_tower`. The server validates the action, updates the game state (e.g., troop attacks tower, tower counter-attacks), and informs both players of the outcome.
    - **Enhanced Mode:**
        - Players can `BUY|troop_name` to spend MANA and add a troop to their available units.
        - Players can `DEPLOY|troop_name|target_tower` to send a purchased troop into combat. This also costs MANA as per `data/specs.json`.
        - The client UI (`printEnhancedState` in `client.go`) displays available troops to buy and current game state including MANA.
        - The server processes these actions, considering MANA costs, troop/tower stats (potentially modified by player level), CRIT chances, and special abilities.
- **Main Interaction Loop:**
    - **Client-Side (`client.go`):**
        - The client continuously listens for messages from the server in a dedicated goroutine (`listenTurnLoop` or similar).
        - It also reads user input from the console (e.g., in `enhancedInputLoop` or `inGameLoop`).
        - User commands are parsed and sent to the server.
    - **Server-Side (`server.go`):**
        - The server's `handleConnection` function for each client reads incoming commands.
        - Commands like `DEPLOY`, `BUY`, etc., trigger corresponding game logic functions (e.g., `handleDeploy`, `handleEnhancedBuy`).
        - The server updates the internal game state (`GameState` or `EnhancedGameState`).
        - After processing an action, the server sends updates to both clients.
- **Receiving Messages:**
    - Clients receive various message types from the server:
        - `ACK|...`: General acknowledgments for commands.
        - `ERR|...`: Error messages.
        - `STATE|<json_state>`: A full game state update, typically in JSON format. The client parses this and updates its display (e.g., `printEnhancedState` in `client.go` for enhanced mode).
        - `ATTACK_RESULT|attacker|defender|damage|remaining_hp`: Specific outcome of an attack.
        - `QUEEN_HEAL|...`: Notification of a special ability usage.
        - `TURN|Your turn!` or `TURN|Wait for your turn...`: Turn indicators for Simple Mode.
        - `GAME_END|winner|reason`: Announces the end of the game and the winner.
    - The client (`client.go`) parses these messages and displays relevant information to the user or updates its internal representation of the game.

## FUTURE DEVELOPMENT
- Add more troop types and abilities.
- Implement advanced matchmaking and ranking.
- Enhance UI/UX for better player experience.
- Support for UDP and WebSocket protocols.

## CONCLUSION
The Text-Based Clash Royale (TCR) project successfully met its goals as a practical net-centric programming exercise. It demonstrated core networking concepts via a Go-based TCP client-server model, implementing features like authentication, lobby management, two game modes, and JSON data persistence.

The project provided significant learning throughout the software development lifecycle, from design and PDU specification to iterative development and GitHub version control. It highlighted the importance of clear client-server communication, error handling, and state management. Go's concurrency and networking features were instrumental in building a responsive server. TCR serves as a playable game and an extendable platform for future enhancements, equipping developers with valuable skills in networked game development and fulfilling its educational objectives.

## REFERENCES
- Go Programming Language: https://golang.org/
- Supercell Clash Royale: https://clashroyale.com/
- GitHub Documentation: https://docs.github.com/
- Canva: https://www.canva.com/
