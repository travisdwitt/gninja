package main

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/gdamore/tcell/v2"
)

const (
	FPS           = 30
	FrameDuration = time.Second / FPS

	PlayerWidth  = 4
	PlayerHeight = 3

	EnemyWidth  = 4
	EnemyHeight = 3
)

type Vec2 struct {
	X, Y float64
}

type Player struct {
	Pos              Vec2
	Vel              Vec2 // Velocity for jumping
	Facing           int  // -1 for left, 1 for right (sprite direction)
	MoveDir          int  // -1 for left, 1 for right, 0 for none (actual movement direction)
	Width            int
	Height           int
	OnGround         bool
	OnPlatform       bool      // true if player is on a platform
	LastOnGroundTime time.Time // Track when player left ground/platform for coyote time
}

type Projectile struct {
	Pos     Vec2
	PrevPos Vec2 // Previous position for swept collision detection
	Dir     int  // -1 for left, 1 for right
	Active  bool
	Frame   int
	IsEnemy bool // true if fired by enemy, false if fired by player
}

type Enemy struct {
	Pos           Vec2
	Vel           Vec2 // Velocity for jumping
	Facing        int  // -1 for left, 1 for right
	Width         int
	Height        int
	Active        bool
	CanShoot      bool          // true if this enemy can fire projectiles
	LastShot      time.Time     // Last time this enemy fired
	NextShotDelay time.Duration // Random delay between 1-3 seconds for next shot
	OnGround      bool
	JumpCooldown  time.Time // Cooldown to prevent constant jumping
}

type DeathParticle struct {
	Pos                     Vec2
	Vel                     Vec2 // Velocity
	Char                    rune // Character to display
	OnGround                bool
	Bounces                 int
	GroundTime              time.Time // When it first hit the ground
	Active                  bool
	EnemyID                 int       // ID of the enemy this particle came from
	IsRed                   bool      // 20% chance to be red
	AngularVel              float64   // Angular velocity for rotation effect
	Angle                   float64   // Current rotation angle
	FallsThrough            bool      // 30% chance to fall through ground
	IsHead                  bool      // true if this is the 'O' head piece
	IsRolling               bool      // true if head piece is rolling
	RollDistance            float64   // Distance to roll
	RollSpeed               float64   // Speed while rolling
	BouncedFromPlatform     bool      // true if particle bounced from a platform
	WasOnPlatform           bool      // true if particle was on a platform (prevents coloring ground beneath)
	HasSplattedFromPlatform bool      // true if particle has already splatted when falling from platform to ground
	LastBloodEmit           time.Time // Last time blood particles were emitted (for continuous emission)
	HasHitGround            bool      // true if particle has hit the ground at least once
}

type BloodParticle struct {
	Pos      Vec2
	Vel      Vec2 // Velocity
	Char     rune // Character to display ('.')
	Active   bool
	Lifetime float64 // Time remaining before disappearing
	EnemyID  int     // ID of the enemy this particle came from
}

// Corpse represents a temporarily mobile body after decapitation
type Corpse struct {
	Pos            Vec2
	Vel            Vec2
	Facing         int
	OnGround       bool
	Active         bool
	EnemyID        int
	EndTime        time.Time
	MoveDir        int
	LastDirChange  time.Time
	DirDuration    time.Duration
	LastBloodEmit  time.Time
	WasOnPlatform  bool
}

type Platform struct {
	X      float64 // Left edge X position
	Y      float64 // Top edge Y position
	Width  float64 // Platform width
	Height float64 // Platform height (usually 1)
}

type Game struct {
	screen             tcell.Screen
	player             Player
	projectiles        []Projectile
	enemies            []Enemy
	deathParticles     []DeathParticle
	corpses            []Corpse
	bloodParticles     []BloodParticle
	platforms          []Platform
	score              int
	enemiesDefeated    int // Track number of enemies defeated
	enemySpawnCounter  int // Counter to track every other enemy for shooting
	gameOver           bool
	inMenu             bool // true when showing main menu
	bloodColorMode     int  // 0=red, 1=green, 2=rainbow, 3=off
	width              int
	height             int
	groundY            int
	redGroundTiles     map[int]int // Tracks which ground tiles are red (key is x position, value is enemy ID)
	redPlatformTiles   map[int]int // Tracks which platform tiles are red (key is platform index + x offset, value is enemy ID)
	nextEnemyID        int         // Counter for assigning unique enemy IDs
	lastFrame          time.Time
	keys               map[tcell.Key]time.Time
	lastShot           time.Time
	menuLastShot       time.Time // Last time menu player fired
	menuLastEnemySpawn time.Time // Last time enemy spawned in menu
}

func NewGame(screen tcell.Screen) *Game {
	width, height := screen.Size()
	groundY := height - 1 // Ground at the bottom of the terminal

	// Create floating platforms
	platforms := make([]Platform, 0)
	// Add 3-5 platforms at various heights
	numPlatforms := 3 + rand.Intn(3) // 3-5 platforms
	platformSpacing := float64(width) / float64(numPlatforms+1)
	groundLevel := float64(groundY)
	for i := 0; i < numPlatforms; i++ {
		platformX := platformSpacing * float64(i+1)
		// Platforms at different heights: 4-12 pixels above ground (reachable with jump)
		// Player can jump about 12 pixels high, so platforms should be within that range
		heightAboveGround := 4.0 + rand.Float64()*8.0      // 4-12 pixels above ground
		platformY := groundLevel - heightAboveGround - 1.0 // -1 to account for platform height
		platformWidth := 12.0 + rand.Float64()*16.0        // 12-28 wide - wider platforms
		platforms = append(platforms, Platform{
			X:      platformX - platformWidth/2,
			Y:      platformY,
			Width:  platformWidth,
			Height: 1.0,
		})
	}

	return &Game{
		screen: screen,
		player: Player{
			Pos:              Vec2{X: float64(width / 2), Y: float64(groundY - PlayerHeight)},
			Vel:              Vec2{X: 0, Y: 0},
			Facing:           1,
			MoveDir:          0,
			Width:            PlayerWidth,
			Height:           PlayerHeight,
			OnGround:         true,
			OnPlatform:       false,
			LastOnGroundTime: time.Now(),
		},
		projectiles:        make([]Projectile, 0),
		enemies:            make([]Enemy, 0),
		deathParticles:     make([]DeathParticle, 0),
		corpses:            make([]Corpse, 0),
		bloodParticles:     make([]BloodParticle, 0),
		platforms:          platforms,
		score:              0,
		gameOver:           false,
		inMenu:             true,
		bloodColorMode:     0, // Start with red
		width:              width,
		height:             height,
		groundY:            groundY,
		redGroundTiles:     make(map[int]int),
		redPlatformTiles:   make(map[int]int),
		nextEnemyID:        1,
		enemiesDefeated:    0,
		enemySpawnCounter:  0,
		lastFrame:          time.Now(),
		keys:               make(map[tcell.Key]time.Time),
		lastShot:           time.Time{},
		menuLastShot:       time.Time{},
		menuLastEnemySpawn: time.Time{},
	}
}

func (g *Game) drawPlayer() {
	x := int(g.player.Pos.X)
	y := int(g.player.Pos.Y)

	style := tcell.StyleDefault.Foreground(tcell.ColorBlue)

	if g.player.Facing == 1 { // Facing right
		g.screen.SetContent(x, y, '~', nil, style)
		g.screen.SetContent(x+1, y, '0', nil, style)
		g.screen.SetContent(x, y+1, '(', nil, style)
		g.screen.SetContent(x+1, y+1, '|', nil, style)
		g.screen.SetContent(x+2, y+1, '\\', nil, style)
		g.screen.SetContent(x, y+2, '/', nil, style)
		g.screen.SetContent(x+1, y+2, ' ', nil, style)
		g.screen.SetContent(x+2, y+2, ')', nil, style)
	} else { // Facing left
		g.screen.SetContent(x, y, ' ', nil, style)
		g.screen.SetContent(x+1, y, '0', nil, style)
		g.screen.SetContent(x+2, y, '~', nil, style)
		g.screen.SetContent(x, y+1, '/', nil, style)
		g.screen.SetContent(x+1, y+1, '|', nil, style)
		g.screen.SetContent(x+2, y+1, ')', nil, style)
		g.screen.SetContent(x, y+2, '(', nil, style)
		g.screen.SetContent(x+1, y+2, ' ', nil, style)
		g.screen.SetContent(x+2, y+2, '\\', nil, style)
	}
}

func (g *Game) drawEnemy(e *Enemy) {
	x := int(e.Pos.X)
	y := int(e.Pos.Y)

	// Use light gray color for all enemies
	style := tcell.StyleDefault.Foreground(tcell.ColorLightGray)

	if e.Facing == 1 { // Facing right
		g.screen.SetContent(x, y, ' ', nil, style)
		g.screen.SetContent(x+1, y, 'O', nil, style)
		g.screen.SetContent(x, y+1, '(', nil, style)
		g.screen.SetContent(x+1, y+1, '|', nil, style)
		g.screen.SetContent(x+2, y+1, '\\', nil, style)
		g.screen.SetContent(x, y+2, '/', nil, style)
		g.screen.SetContent(x+1, y+2, ' ', nil, style)
		g.screen.SetContent(x+2, y+2, ')', nil, style)
	} else { // Facing left
		g.screen.SetContent(x, y, ' ', nil, style)
		g.screen.SetContent(x+1, y, 'O', nil, style)
		g.screen.SetContent(x+2, y, ' ', nil, style)
		g.screen.SetContent(x, y+1, '/', nil, style)
		g.screen.SetContent(x+1, y+1, '|', nil, style)
		g.screen.SetContent(x+2, y+1, ')', nil, style)
		g.screen.SetContent(x, y+2, '(', nil, style)
		g.screen.SetContent(x+1, y+2, ' ', nil, style)
		g.screen.SetContent(x+2, y+2, '\\', nil, style)
	}
}

func (g *Game) drawCorpse(c *Corpse) {
	x := int(c.Pos.X)
	y := int(c.Pos.Y)
	style := tcell.StyleDefault.Foreground(tcell.ColorLightGray)
	// Draw a headless, shuffling corpse
	if c.Facing == 1 {
		// Facing right: remove head and show (|\ on row 1, / ) on row 2
		g.screen.SetContent(x, y, ' ', nil, style)
		g.screen.SetContent(x+1, y, ' ', nil, style)
		g.screen.SetContent(x+2, y, ' ', nil, style)

		g.screen.SetContent(x, y+1, '(', nil, style)
		g.screen.SetContent(x+1, y+1, '|', nil, style)
		g.screen.SetContent(x+2, y+1, '\\', nil, style)

		g.screen.SetContent(x, y+2, '/', nil, style)
		g.screen.SetContent(x+1, y+2, ' ', nil, style)
		g.screen.SetContent(x+2, y+2, ')', nil, style)
	} else {
		// Facing left: remove head and show /|) on row 1, ( \ on row 2
		g.screen.SetContent(x, y, ' ', nil, style)
		g.screen.SetContent(x+1, y, ' ', nil, style)
		g.screen.SetContent(x+2, y, ' ', nil, style)

		g.screen.SetContent(x, y+1, '/', nil, style)
		g.screen.SetContent(x+1, y+1, '|', nil, style)
		g.screen.SetContent(x+2, y+1, ')', nil, style)

		g.screen.SetContent(x, y+2, '(', nil, style)
		g.screen.SetContent(x+1, y+2, ' ', nil, style)
		g.screen.SetContent(x+2, y+2, '\\', nil, style)
	}
}

func (g *Game) drawProjectile(p *Projectile) {
	x := int(p.Pos.X)
	y := int(p.Pos.Y)

	// Use cyan color for enemy projectiles, yellow for player projectiles
	var style tcell.Style
	if p.IsEnemy {
		cyanColor := tcell.Color(51) // Cyan in 256-color palette
		style = tcell.StyleDefault.Foreground(cyanColor)
	} else {
		style = tcell.StyleDefault.Foreground(tcell.ColorYellow)
	}

	// Animate between '-' and '+'
	if p.Frame%2 == 0 {
		g.screen.SetContent(x, y, '-', nil, style)
	} else {
		g.screen.SetContent(x, y, '+', nil, style)
	}
}

func (g *Game) drawGround() {
	// Use horizontal line character for ground (less tall than full block)
	groundChar := '━' // Heavy horizontal line (U+2501)
	// Dark gray color (using color index 8 from standard palette, or 240 for lighter dark gray)
	darkGray := tcell.Color(240) // Dark gray in 256-color palette

	for x := 0; x < g.width; x++ {
		// Use blood color mode if tile is marked, otherwise dark gray
		if _, isMarked := g.redGroundTiles[x]; isMarked {
			// If blood is off, use dark gray
			if g.bloodColorMode == 3 {
				style := tcell.StyleDefault.Foreground(darkGray)
				g.screen.SetContent(x, g.groundY, groundChar, nil, style)
			} else {
				var style tcell.Style

				switch g.bloodColorMode {
				case 0: // Red
					style = tcell.StyleDefault.Foreground(tcell.ColorRed)
				case 1: // Green
					style = tcell.StyleDefault.Foreground(tcell.ColorGreen)
				case 2: // Rainbow (deterministic color based on position)
					// Use position to create rainbow effect (no time component)
					colors := []tcell.Color{
						tcell.ColorRed,
						tcell.ColorYellow,
						tcell.ColorGreen,
						tcell.ColorBlue,
						tcell.ColorLightGray,
						tcell.ColorWhite,
					}
					colorIndex := x % len(colors)
					style = tcell.StyleDefault.Foreground(colors[colorIndex])
				default:
					style = tcell.StyleDefault.Foreground(tcell.ColorRed)
				}

				g.screen.SetContent(x, g.groundY, groundChar, nil, style)
			}
		} else {
			style := tcell.StyleDefault.Foreground(darkGray)
			g.screen.SetContent(x, g.groundY, groundChar, nil, style)
		}
	}
}

func (g *Game) drawPlatforms() {
	platformChar := '━'                                      // Same as ground
	cyanColor := tcell.Color(51)                             // Cyan in 256-color palette
	defaultStyle := tcell.StyleDefault.Foreground(cyanColor) // Cyan color for platforms

	for platformIndex, platform := range g.platforms {
		startX := int(platform.X)
		endX := int(platform.X + platform.Width)
		y := int(platform.Y)

		for x := startX; x < endX && x < g.width; x++ {
			if x >= 0 {
				// Check if this tile is marked red
				key := platformIndex*10000 + x
				var style tcell.Style
				if _, isMarked := g.redPlatformTiles[key]; isMarked {
					// Use blood color mode if tile is marked
					if g.bloodColorMode == 3 {
						style = defaultStyle
					} else {
						switch g.bloodColorMode {
						case 0: // Red
							style = tcell.StyleDefault.Foreground(tcell.ColorRed)
						case 1: // Green
							style = tcell.StyleDefault.Foreground(tcell.ColorGreen)
						case 2: // Rainbow
							colors := []tcell.Color{
								tcell.ColorRed,
								tcell.ColorYellow,
								tcell.ColorGreen,
								tcell.ColorBlue,
								tcell.ColorLightGray,
								tcell.ColorWhite,
							}
							colorIndex := x % len(colors)
							style = tcell.StyleDefault.Foreground(colors[colorIndex])
						default:
							style = tcell.StyleDefault.Foreground(tcell.ColorRed)
						}
					}
				} else {
					style = defaultStyle
				}
				g.screen.SetContent(x, y, platformChar, nil, style)
			}
		}
	}
}

func (g *Game) drawScore() {
	style := tcell.StyleDefault.Foreground(tcell.ColorWhite)
	scoreText := fmt.Sprintf("Score: %d", g.score)
	for i, r := range scoreText {
		g.screen.SetContent(i, 0, r, nil, style)
	}
}

func (g *Game) drawMenu() {
	// Title "GNinja" in green, centered
	title := "GNinja"
	titleX := (g.width - len(title)) / 2
	titleY := g.height/2 - 2

	greenStyle := tcell.StyleDefault.Foreground(tcell.ColorGreen)
	for i, r := range title {
		g.screen.SetContent(titleX+i, titleY, r, nil, greenStyle)
	}

	// "Press Space to start" below title
	startText := "Press Space to start"
	startX := (g.width - len(startText)) / 2
	startY := titleY + 2

	for i, r := range startText {
		g.screen.SetContent(startX+i, startY, r, nil, tcell.StyleDefault)
	}

	// Show current blood color mode
	var modeText string
	switch g.bloodColorMode {
	case 0:
		modeText = "Blood: Red (Press TAB to change)"
	case 1:
		modeText = "Blood: Green (Press TAB to change)"
	case 2:
		modeText = "Blood: Rainbow (Press TAB to change)"
	case 3:
		modeText = "Blood: Off (Press TAB to change)"
	}
	modeX := (g.width - len(modeText)) / 2
	modeY := startY + 2

	for i, r := range modeText {
		g.screen.SetContent(modeX+i, modeY, r, nil, tcell.StyleDefault)
	}
}

func (g *Game) drawGameOver() {
	// Draw game over text at the top center, not covering the game
	style := tcell.StyleDefault.Foreground(tcell.ColorRed).Bold(true)
	text := "GAME OVER"
	startX := (g.width - len(text)) / 2
	startY := 2 // Near the top, below score

	for i, r := range text {
		g.screen.SetContent(startX+i, startY, r, nil, style)
	}

	style = tcell.StyleDefault.Foreground(tcell.ColorWhite)
	instructions := []string{
		"Press ENTER to restart",
		"Press ESC to exit",
	}

	for i, line := range instructions {
		lineX := (g.width - len(line)) / 2
		for j, r := range line {
			g.screen.SetContent(lineX+j, startY+2+i, r, nil, style)
		}
	}
}

func (g *Game) updatePlayer(deltaTime float64) {
	groundSpeed := 50.0 // pixels per second - reduced ground movement speed
	airSpeed := 45.0    // pixels per second - improved air control (closer to ground speed)
	gravity := 300.0    // pixels per second squared
	jumpSpeed := -85.0  // upward velocity for jump (reduced for lower jump)
	groundY := float64(g.groundY - PlayerHeight)

	now := time.Now()
	keyTimeout := 150 * time.Millisecond // Increased timeout for smoother, more responsive controls

	// Choose speed based on whether player is on ground or in air
	speed := groundSpeed
	if !g.player.OnGround {
		speed = airSpeed // Improved air control
	}

	// Handle smooth movement based on key states - works both on ground and in air
	// Separate facing direction from movement direction
	leftPressed := false
	rightPressed := false
	if lastPress, ok := g.keys[tcell.KeyLeft]; ok && now.Sub(lastPress) < keyTimeout {
		leftPressed = true
		g.player.Facing = -1 // Update facing immediately when key is pressed
	}
	if lastPress, ok := g.keys[tcell.KeyRight]; ok && now.Sub(lastPress) < keyTimeout {
		rightPressed = true
		g.player.Facing = 1 // Update facing immediately when key is pressed
	}

	// Update movement direction - only move if key is actively held
	// Movement happens continuously while key is held, not just on press
	g.player.MoveDir = 0
	if leftPressed {
		g.player.MoveDir = -1
		g.player.Pos.X -= speed * deltaTime
	}
	if rightPressed {
		g.player.MoveDir = 1
		g.player.Pos.X += speed * deltaTime
	}

	// Handle jumping - improved diagonal jumps with coyote time
	coyoteTime := 100 * time.Millisecond // Allow jumping slightly after leaving ground/platform
	canJump := g.player.OnGround || g.player.OnPlatform ||
		(!g.player.OnGround && !g.player.OnPlatform && time.Since(g.player.LastOnGroundTime) < coyoteTime)

	if lastPress, ok := g.keys[tcell.KeyUp]; ok && now.Sub(lastPress) < keyTimeout {
		if canJump {
			g.player.Vel.Y = jumpSpeed
			g.player.OnGround = false
			g.player.OnPlatform = false
		}
	}

	// Apply gravity if not on ground
	if !g.player.OnGround {
		g.player.Vel.Y += gravity * deltaTime
	}

	// Update vertical position
	g.player.Pos.Y += g.player.Vel.Y * deltaTime

	// Check platform collisions first
	onPlatform := false
	platformTopY := 0.0
	for _, platform := range g.platforms {
		// Check if player is above platform and within horizontal bounds
		if g.player.Pos.X < platform.X+platform.Width &&
			g.player.Pos.X+float64(g.player.Width) > platform.X &&
			g.player.Pos.Y < platform.Y+platform.Height &&
			g.player.Pos.Y+float64(g.player.Height) > platform.Y {
			// Player is colliding with platform
			// If falling down onto platform, land on top
			if g.player.Vel.Y > 0 && g.player.Pos.Y < platform.Y {
				platformTopY = platform.Y - float64(g.player.Height)
				g.player.Pos.Y = platformTopY
				g.player.Vel.Y = 0
				g.player.OnGround = false
				g.player.OnPlatform = true
				g.player.LastOnGroundTime = time.Now()
				onPlatform = true
				break
			}
		}
	}

	// Check ground collision only if not on a platform
	if !onPlatform {
		if g.player.Pos.Y >= groundY {
			g.player.Pos.Y = groundY
			if g.player.Vel.Y > 0 {
				g.player.Vel.Y = 0
				g.player.OnGround = true
				g.player.OnPlatform = false
				g.player.LastOnGroundTime = time.Now()
			}
		} else {
			// Check if player is falling through platforms (not on top)
			if g.player.OnGround || g.player.OnPlatform {
				g.player.LastOnGroundTime = time.Now()
			}
			g.player.OnGround = false
			g.player.OnPlatform = false
		}
	}

	// Keep player in bounds horizontally
	if g.player.Pos.X < 0 {
		g.player.Pos.X = 0
	}
	if g.player.Pos.X+float64(g.player.Width) > float64(g.width) {
		g.player.Pos.X = float64(g.width - g.player.Width)
	}
}

func (g *Game) updateProjectiles(deltaTime float64) {
	playerProjectileSpeed := 200.0 // pixels per second
	enemyProjectileSpeed := 80.0   // pixels per second - even slower for easier dodging

	// When game over, remove all projectiles
	if g.gameOver {
		g.projectiles = make([]Projectile, 0)
		return
	}

	for i := range g.projectiles {
		if !g.projectiles[i].Active {
			continue
		}

		// Use different speeds for player and enemy projectiles
		speed := playerProjectileSpeed
		if g.projectiles[i].IsEnemy {
			speed = enemyProjectileSpeed
		}

		// Store previous position for swept collision detection
		g.projectiles[i].PrevPos = g.projectiles[i].Pos
		g.projectiles[i].Pos.X += float64(g.projectiles[i].Dir) * speed * deltaTime
		g.projectiles[i].Frame++

		// Remove projectiles that go off screen
		if g.projectiles[i].Pos.X < 0 || g.projectiles[i].Pos.X > float64(g.width) {
			g.projectiles[i].Active = false
		}
	}

	// Clean up inactive projectiles
	active := g.projectiles[:0]
	for i := range g.projectiles {
		if g.projectiles[i].Active {
			active = append(active, g.projectiles[i])
		}
	}
	g.projectiles = active
}

func (g *Game) updateEnemies(deltaTime float64) {
	baseSpeed := 20.0  // pixels per second - slower
	gravity := 300.0   // pixels per second squared (same as player)
	jumpSpeed := -85.0 // Same as player jump speed
	groundY := float64(g.groundY - EnemyHeight)

	for i := range g.enemies {
		if !g.enemies[i].Active {
			continue
		}

		speed := baseSpeed
		minDistance := 30.0 // Minimum distance shooting enemies try to maintain

		if g.gameOver {
			// When game over, make enemies walk off screen (toward nearest edge)
			screenCenter := float64(g.width) / 2.0
			dx := g.enemies[i].Pos.X - screenCenter
			if dx > 0 {
				// Move right off screen
				g.enemies[i].Facing = 1
				g.enemies[i].Pos.X += speed * deltaTime
			} else {
				// Move left off screen
				g.enemies[i].Facing = -1
				g.enemies[i].Pos.X -= speed * deltaTime
			}
		} else {
			// Handle movement based on whether enemy can shoot
			dx := g.player.Pos.X - g.enemies[i].Pos.X
			distance := math.Abs(dx)

			if g.enemies[i].CanShoot {
				// Shooting enemies try to maintain minimum distance
				if distance < minDistance {
					// Too close, move away from player
					if dx > 0 {
						g.enemies[i].Facing = -1
						g.enemies[i].Pos.X -= speed * deltaTime
					} else {
						g.enemies[i].Facing = 1
						g.enemies[i].Pos.X += speed * deltaTime
					}
				} else if distance > minDistance+10.0 {
					// Too far, move towards player
					if dx > 0 {
						g.enemies[i].Facing = 1
						g.enemies[i].Pos.X += speed * deltaTime
					} else {
						g.enemies[i].Facing = -1
						g.enemies[i].Pos.X -= speed * deltaTime
					}
				}
				// If at good distance, don't move horizontally
			} else {
				// Non-shooting enemies always move towards player
				if dx > 0 {
					g.enemies[i].Facing = 1
					g.enemies[i].Pos.X += speed * deltaTime
				} else {
					g.enemies[i].Facing = -1
					g.enemies[i].Pos.X -= speed * deltaTime
				}
			}

			// Handle enemy shooting
			if g.enemies[i].CanShoot && !g.gameOver {
				now := time.Now()
				if g.enemies[i].LastShot.IsZero() {
					// First shot - set initial delay
					g.enemies[i].NextShotDelay = time.Duration(1.0+rand.Float64()*2.0) * time.Second
					g.enemies[i].LastShot = now
				} else if now.Sub(g.enemies[i].LastShot) >= g.enemies[i].NextShotDelay {
					// Time to shoot
					p := Projectile{
						Pos:     Vec2{X: g.enemies[i].Pos.X + float64(g.enemies[i].Width/2), Y: g.enemies[i].Pos.Y + float64(g.enemies[i].Height/2)},
						PrevPos: Vec2{X: g.enemies[i].Pos.X + float64(g.enemies[i].Width/2), Y: g.enemies[i].Pos.Y + float64(g.enemies[i].Height/2)},
						Dir:     g.enemies[i].Facing,
						Active:  true,
						Frame:   0,
						IsEnemy: true,
					}
					g.projectiles = append(g.projectiles, p)
					g.enemies[i].LastShot = now
					// Set next shot delay (1-3 seconds)
					g.enemies[i].NextShotDelay = time.Duration(1.0+rand.Float64()*2.0) * time.Second
				}
			}

			// Check if enemy should jump to reach player or platform
			// Jump if player is significantly higher and enemy is on ground
			dy := g.player.Pos.Y - g.enemies[i].Pos.Y
			if g.enemies[i].OnGround && dy < -10.0 && time.Since(g.enemies[i].JumpCooldown) > 1*time.Second {
				// Player is above, try to jump
				g.enemies[i].Vel.Y = jumpSpeed
				g.enemies[i].OnGround = false
				g.enemies[i].JumpCooldown = time.Now()
			} else {
				// Check if there's a platform nearby that the enemy should jump to
				for _, platform := range g.platforms {
					platformY := platform.Y - float64(EnemyHeight)
					// If platform is above enemy and within reasonable distance
					if platformY < g.enemies[i].Pos.Y-5.0 &&
						math.Abs(g.enemies[i].Pos.X-(platform.X+platform.Width/2)) < 30.0 &&
						g.enemies[i].OnGround &&
						time.Since(g.enemies[i].JumpCooldown) > 1*time.Second {
						g.enemies[i].Vel.Y = jumpSpeed
						g.enemies[i].OnGround = false
						g.enemies[i].JumpCooldown = time.Now()
						break
					}
				}
			}
		}

		// Apply gravity if not on ground
		if !g.enemies[i].OnGround {
			g.enemies[i].Vel.Y += gravity * deltaTime
		}

		// Update vertical position
		g.enemies[i].Pos.Y += g.enemies[i].Vel.Y * deltaTime

		// Check platform collisions
		onPlatform := false
		for _, platform := range g.platforms {
			// Check if enemy is above platform and within horizontal bounds
			if g.enemies[i].Pos.X < platform.X+platform.Width &&
				g.enemies[i].Pos.X+float64(g.enemies[i].Width) > platform.X &&
				g.enemies[i].Pos.Y < platform.Y+platform.Height &&
				g.enemies[i].Pos.Y+float64(g.enemies[i].Height) > platform.Y {
				// Enemy is colliding with platform
				// If falling down onto platform, land on top
				if g.enemies[i].Vel.Y > 0 && g.enemies[i].Pos.Y < platform.Y {
					platformTopY := platform.Y - float64(g.enemies[i].Height)
					g.enemies[i].Pos.Y = platformTopY
					g.enemies[i].Vel.Y = 0
					g.enemies[i].OnGround = true
					onPlatform = true
					break
				}
			}
		}

		// Check ground collision only if not on a platform
		if !onPlatform {
			if g.enemies[i].Pos.Y >= groundY {
				g.enemies[i].Pos.Y = groundY
				if g.enemies[i].Vel.Y > 0 {
					g.enemies[i].Vel.Y = 0
					g.enemies[i].OnGround = true
				}
			} else {
				g.enemies[i].OnGround = false
			}
		}

		// Remove enemies that go off screen
		if g.enemies[i].Pos.X < -float64(EnemyWidth) || g.enemies[i].Pos.X > float64(g.width) {
			g.enemies[i].Active = false
		}
	}

	// Clean up inactive enemies
	active := g.enemies[:0]
	for i := range g.enemies {
		if g.enemies[i].Active {
			active = append(active, g.enemies[i])
		}
	}
	g.enemies = active

	// Don't spawn enemies if game is over
	if g.gameOver {
		return
	}

	// Calculate spawn rate based on enemies defeated (increases every 5 enemies)
	baseSpawnRate := 0.008                                    // 0.8% base chance per frame (lowered from 2%)
	spawnRateIncrease := float64(g.enemiesDefeated/5) * 0.005 // +0.5% per 5 enemies defeated
	spawnRate := baseSpawnRate + spawnRateIncrease
	if spawnRate > 0.05 {
		spawnRate = 0.05 // Cap at 5% max
	}

	// Spawn new enemies randomly
	if rand.Float64() < spawnRate {
		var e Enemy

		// Every other enemy can shoot
		canShoot := (g.enemySpawnCounter%2 == 1)
		g.enemySpawnCounter++

		if rand.Float64() < 0.5 {
			// Spawn from left
			e = Enemy{
				Pos:           Vec2{X: -float64(EnemyWidth), Y: float64(g.groundY - EnemyHeight)},
				Vel:           Vec2{X: 0, Y: 0},
				Facing:        1,
				Width:         EnemyWidth,
				Height:        EnemyHeight,
				Active:        true,
				CanShoot:      canShoot,
				LastShot:      time.Time{},
				NextShotDelay: 0,
				OnGround:      true,
				JumpCooldown:  time.Time{},
			}
		} else {
			// Spawn from right
			e = Enemy{
				Pos:           Vec2{X: float64(g.width), Y: float64(g.groundY - EnemyHeight)},
				Vel:           Vec2{X: 0, Y: 0},
				Facing:        -1,
				Width:         EnemyWidth,
				Height:        EnemyHeight,
				Active:        true,
				CanShoot:      canShoot,
				LastShot:      time.Time{},
				NextShotDelay: 0,
				OnGround:      true,
				JumpCooldown:  time.Time{},
			}
		}
		g.enemies = append(g.enemies, e)
	}
}

func (g *Game) createPlayerDeathParticles() {
	// Assign a unique enemy ID for player particles (use 0 for player)
	enemyID := 0

	// Extract actual player sprite characters based on facing direction
	type SpritePiece struct {
		char rune
		x    int
		y    int
	}

	var pieces []SpritePiece

	if g.player.Facing == 1 { // Facing right
		pieces = []SpritePiece{
			{'~', 0, 0},
			{'0', 1, 0},
			{'(', 0, 1},
			{'|', 1, 1},
			{'\\', 2, 1},
			{'/', 0, 2},
			{')', 2, 2},
		}
	} else { // Facing left
		pieces = []SpritePiece{
			{'0', 1, 0},
			{'~', 2, 0},
			{'/', 0, 1},
			{'|', 1, 1},
			{')', 2, 1},
			{'(', 0, 2},
			{'\\', 2, 2},
		}
	}

	// Create particles from actual player pieces
	for _, piece := range pieces {
		// 20% chance to be red
		isRed := rand.Float64() < 0.2
		// 30% chance to fall through ground
		fallsThrough := rand.Float64() < 0.3

		// Check if this is the head piece ('0')
		isHead := (piece.char == '0')
		isRolling := false
		rollDistance := 0.0
		rollSpeed := 0.0

		// Head piece has 5% chance to roll after bouncing
		if isHead && rand.Float64() < 0.05 {
			isRolling = true
			rollDistance = 20.0 + rand.Float64()*30.0 // Roll 20-50 pixels
			rollSpeed = 40.0 + rand.Float64()*20.0    // 40-60 pixels/sec rolling speed
			// Random direction for rolling
			if rand.Float64() < 0.5 {
				rollSpeed = -rollSpeed // Roll left
			}
		}

		// More dynamic velocities - varied directions and speeds (reduced for less explosive effect)
		angle := rand.Float64() * 2 * 3.14159 // Random angle in radians
		speed := 20.0 + rand.Float64()*30.0   // 20-50 pixels/sec (reduced from 30-80)
		velX := math.Cos(angle) * speed
		velY := -15.0 - rand.Float64()*25.0 + math.Sin(angle)*speed*0.5 // Upward bias with variation (reduced)

		// Angular velocity for rotation effect
		angularVel := (rand.Float64() - 0.5) * 360.0 // -180 to 180 degrees per second

		particle := DeathParticle{
			Pos:                     Vec2{X: g.player.Pos.X + float64(piece.x), Y: g.player.Pos.Y + float64(piece.y)},
			Vel:                     Vec2{X: velX, Y: velY},
			Char:                    piece.char,
			OnGround:                false,
			Bounces:                 0,
			GroundTime:              time.Time{},
			Active:                  true,
			EnemyID:                 enemyID,
			IsRed:                   isRed,
			AngularVel:              angularVel,
			Angle:                   0,
			FallsThrough:            fallsThrough,
			IsHead:                  isHead,
			IsRolling:               isRolling,
			RollDistance:            rollDistance,
			RollSpeed:               rollSpeed,
			BouncedFromPlatform:     false,
			WasOnPlatform:           false,
			HasSplattedFromPlatform: false,
			LastBloodEmit:           time.Now(),
			HasHitGround:            false,
		}
		g.deathParticles = append(g.deathParticles, particle)

		// Emit blood particles when player is hit (initial burst)
		initialSpeed := math.Sqrt(velX*velX + velY*velY)
		intensity := math.Min(initialSpeed/80.0, 1.0)
		g.emitBloodFromParticle(&g.deathParticles[len(g.deathParticles)-1], intensity, true)
	}
}

func (g *Game) createDeathParticles(e *Enemy) {
	// 5% chance to decapitate: head pops off and body becomes mobile corpse
	if rand.Float64() < 0.1 {
		g.createDecap(e)
		return
	}
	// Assign a unique enemy ID for this enemy's particles
	enemyID := g.nextEnemyID
	g.nextEnemyID++

	// Check if enemy is on a platform
	wasOnPlatform := false
	for _, platform := range g.platforms {
		// Check if enemy is on top of platform
		enemyBottomY := e.Pos.Y + float64(e.Height)
		if e.Pos.X < platform.X+platform.Width &&
			e.Pos.X+float64(e.Width) > platform.X &&
			math.Abs(enemyBottomY-platform.Y) < 2.0 { // Within 2 pixels of platform top
			wasOnPlatform = true
			break
		}
	}

	// Extract actual enemy sprite characters based on facing direction
	type SpritePiece struct {
		char rune
		x    int
		y    int
	}

	var pieces []SpritePiece

	if e.Facing == 1 { // Facing right
		pieces = []SpritePiece{
			{'O', 1, 0},
			{'(', 0, 1},
			{'|', 1, 1},
			{'\\', 2, 1},
			{'/', 0, 2},
			{')', 2, 2},
		}
	} else { // Facing left
		pieces = []SpritePiece{
			{'O', 1, 0},
			{'/', 0, 1},
			{'|', 1, 1},
			{')', 2, 1},
			{'(', 0, 2},
			{'\\', 2, 2},
		}
	}

	// Create particles from actual enemy pieces
	for _, piece := range pieces {
		// 20% chance to be red
		isRed := rand.Float64() < 0.2
		// 30% chance to fall through ground
		fallsThrough := rand.Float64() < 0.3

		// Check if this is the head piece ('O')
		isHead := (piece.char == 'O')
		isRolling := false
		rollDistance := 0.0
		rollSpeed := 0.0

		// Head piece has 5% chance to roll after bouncing
		if isHead && rand.Float64() < 0.05 {
			isRolling = true
			rollDistance = 20.0 + rand.Float64()*30.0 // Roll 20-50 pixels
			rollSpeed = 40.0 + rand.Float64()*20.0    // 40-60 pixels/sec rolling speed
			// Random direction for rolling
			if rand.Float64() < 0.5 {
				rollSpeed = -rollSpeed // Roll left
			}
		}

		// More dynamic velocities - varied directions and speeds (reduced for less explosive effect)
		angle := rand.Float64() * 2 * 3.14159 // Random angle in radians
		speed := 20.0 + rand.Float64()*30.0   // 20-50 pixels/sec (reduced from 30-80)
		velX := math.Cos(angle) * speed
		velY := -15.0 - rand.Float64()*25.0 + math.Sin(angle)*speed*0.5 // Upward bias with variation (reduced)

		// Angular velocity for rotation effect
		angularVel := (rand.Float64() - 0.5) * 360.0 // -180 to 180 degrees per second

		particle := DeathParticle{
			Pos:                     Vec2{X: e.Pos.X + float64(piece.x), Y: e.Pos.Y + float64(piece.y)},
			Vel:                     Vec2{X: velX, Y: velY},
			Char:                    piece.char,
			OnGround:                false,
			Bounces:                 0,
			GroundTime:              time.Time{},
			Active:                  true,
			EnemyID:                 enemyID,
			IsRed:                   isRed,
			AngularVel:              angularVel,
			Angle:                   0,
			FallsThrough:            fallsThrough,
			IsHead:                  isHead,
			IsRolling:               isRolling,
			RollDistance:            rollDistance,
			RollSpeed:               rollSpeed,
			BouncedFromPlatform:     false,
			WasOnPlatform:           wasOnPlatform,
			HasSplattedFromPlatform: false,
			LastBloodEmit:           time.Now(),
			HasHitGround:            false,
		}
		g.deathParticles = append(g.deathParticles, particle)

		// Emit blood particles when enemy is hit (initial burst)
		// More intense for faster-moving pieces
		initialSpeed := math.Sqrt(velX*velX + velY*velY)
		intensity := math.Min(initialSpeed/80.0, 1.0) // Scale intensity by speed
		g.emitBloodFromParticle(&g.deathParticles[len(g.deathParticles)-1], intensity, true)
	}

}

// createDeathParticlesAt splits a body at a given position/facing using the provided enemyID
func (g *Game) createDeathParticlesAt(pos Vec2, facing int, enemyID int, wasOnPlatform bool) {
	// Extract actual enemy sprite characters based on facing direction
	type SpritePiece struct {
		char rune
		x    int
		y    int
	}

	var pieces []SpritePiece
	if facing == 1 { // Facing right
		pieces = []SpritePiece{
			{'O', 1, 0},
			{'(', 0, 1},
			{'|', 1, 1},
			{'\\', 2, 1},
			{'/', 0, 2},
			{')', 2, 2},
		}
	} else {
		pieces = []SpritePiece{
			{'O', 1, 0},
			{'/', 0, 1},
			{'|', 1, 1},
			{')', 2, 1},
			{'(', 0, 2},
			{'\\', 2, 2},
		}
	}

	for _, piece := range pieces {
		// 20% chance to be red
		isRed := rand.Float64() < 0.2
		// 30% chance to fall through ground
		fallsThrough := rand.Float64() < 0.3

		// Check if this is the head piece
		isHead := (piece.char == 'O')
		isRolling := false
		rollDistance := 0.0
		rollSpeed := 0.0

		// Head piece may roll if it lands
		if isHead && rand.Float64() < 0.05 {
			isRolling = true
			rollDistance = 20.0 + rand.Float64()*30.0
			rollSpeed = 40.0 + rand.Float64()*20.0
			if rand.Float64() < 0.5 {
				rollSpeed = -rollSpeed
			}
		}

		angle := rand.Float64() * 2 * 3.14159
		speed := 20.0 + rand.Float64()*30.0
		velX := math.Cos(angle) * speed
		velY := -15.0 - rand.Float64()*25.0 + math.Sin(angle)*speed*0.5

		angularVel := (rand.Float64() - 0.5) * 360.0

		particle := DeathParticle{
			Pos:                     Vec2{X: pos.X + float64(piece.x), Y: pos.Y + float64(piece.y)},
			Vel:                     Vec2{X: velX, Y: velY},
			Char:                    piece.char,
			OnGround:                false,
			Bounces:                 0,
			GroundTime:              time.Time{},
			Active:                  true,
			EnemyID:                 enemyID,
			IsRed:                   isRed,
			AngularVel:              angularVel,
			Angle:                   0,
			FallsThrough:            fallsThrough,
			IsHead:                  isHead,
			IsRolling:               isRolling,
			RollDistance:            rollDistance,
			RollSpeed:               rollSpeed,
			BouncedFromPlatform:     false,
			WasOnPlatform:           wasOnPlatform,
			HasSplattedFromPlatform: false,
			LastBloodEmit:           time.Now(),
			HasHitGround:            false,
		}
		g.deathParticles = append(g.deathParticles, particle)

		initialSpeed := math.Sqrt(velX*velX + velY*velY)
		intensity := math.Min(initialSpeed/80.0, 1.0)
		g.emitBloodFromParticle(&g.deathParticles[len(g.deathParticles)-1], intensity, true)
	}
}

// createDecap handles the special decapitation case: spawn a rolling head and a mobile corpse
func (g *Game) createDecap(e *Enemy) {
	// Reserve an enemy ID for corpse and head
	enemyID := g.nextEnemyID
	g.nextEnemyID++

	// Determine if enemy was on a platform
	wasOnPlatform := false
	for _, platform := range g.platforms {
		enemyBottomY := e.Pos.Y + float64(e.Height)
		if e.Pos.X < platform.X+platform.Width &&
			e.Pos.X+float64(e.Width) > platform.X &&
			math.Abs(enemyBottomY-platform.Y) < 2.0 {
			wasOnPlatform = true
			break
		}
	}

	// Create a single head particle that pops off and rolls
	pieceX := 1
	pieceY := 0
	if e.Facing != 1 {
		// same offsets regardless of facing in this sprite
	}

	isHead := true
	isRolling := true
	rollDistance := 20.0 + rand.Float64()*30.0
	rollSpeed := 40.0 + rand.Float64()*20.0
	if rand.Float64() < 0.5 {
		rollSpeed = -rollSpeed
	}

	angle := rand.Float64() * 2 * 3.14159
	speed := 20.0 + rand.Float64()*30.0
	velX := math.Cos(angle) * speed
	velY := -15.0 - rand.Float64()*25.0 + math.Sin(angle)*speed*0.5

	angularVel := (rand.Float64() - 0.5) * 360.0

	head := DeathParticle{
		Pos:          Vec2{X: e.Pos.X + float64(pieceX), Y: e.Pos.Y + float64(pieceY)},
		Vel:          Vec2{X: velX, Y: velY},
		Char:         'O',
		OnGround:     false,
		Bounces:      0,
		GroundTime:   time.Time{},
		Active:       true,
		EnemyID:      enemyID,
		IsRed:        true,
		AngularVel:   angularVel,
		Angle:        0,
		FallsThrough: false,
		IsHead:       isHead,
		IsRolling:    isRolling,
		RollDistance: rollDistance,
		RollSpeed:    rollSpeed,
		LastBloodEmit: time.Now(),
	}
	g.deathParticles = append(g.deathParticles, head)
	// Emit an initial burst from the head pop
	initialSpeed := math.Sqrt(velX*velX + velY*velY)
	intensity := math.Min(initialSpeed/80.0, 1.0)
	g.emitBloodFromParticle(&g.deathParticles[len(g.deathParticles)-1], intensity, true)

	// Create a mobile corpse that will run back and forth while squirting blood
	duration := 2*time.Second + time.Duration(rand.Intn(2000))*time.Millisecond // 2-4s
	moveDir := 1
	if rand.Float64() < 0.5 {
		moveDir = -1
	}
	corp := Corpse{
		Pos:           Vec2{X: e.Pos.X, Y: e.Pos.Y},
		Vel:           e.Vel,
		Facing:        e.Facing,
		OnGround:      false,
		Active:        true,
		EnemyID:       enemyID,
		EndTime:       time.Now().Add(duration),
		MoveDir:       moveDir,
		LastDirChange: time.Now(),
		DirDuration:   time.Duration(200+rand.Intn(600)) * time.Millisecond,
		LastBloodEmit: time.Now(),
		WasOnPlatform: wasOnPlatform,
	}
	g.corpses = append(g.corpses, corp)
}

// updateCorpses moves mobile corpses and makes them squirt blood; when expired they break apart
func (g *Game) updateCorpses(deltaTime float64) {
	if len(g.corpses) == 0 {
		return
	}

	now := time.Now()
	gravity := 300.0
	groundY := float64(g.groundY - EnemyHeight)
	for i := range g.corpses {
		c := &g.corpses[i]
		if !c.Active {
			continue
		}

		// Possibly change direction after DirDuration
		if now.Sub(c.LastDirChange) >= c.DirDuration {
			c.LastDirChange = now
			c.DirDuration = time.Duration(200+rand.Intn(600)) * time.Millisecond
			// Randomly flip direction with 50% chance
			if rand.Float64() < 0.5 {
				c.MoveDir = -c.MoveDir
			}
		}

		// Move corpse horizontally
		speed := 25.0
		c.Pos.X += float64(c.MoveDir) * speed * deltaTime

		// Apply gravity and vertical movement
		c.Vel.Y += gravity * deltaTime
		c.Pos.Y += c.Vel.Y * deltaTime

		// Check platform collisions
		onPlatform := false
		for _, platform := range g.platforms {
			particleBottomY := c.Pos.Y + float64(EnemyHeight)
			if c.Pos.X < platform.X+platform.Width &&
				c.Pos.X+float64(EnemyWidth) > platform.X &&
				particleBottomY >= platform.Y &&
				particleBottomY <= platform.Y+platform.Height+1.0 {
				// Land on platform
				if c.Vel.Y > 0 && c.Pos.Y < platform.Y {
					platformTopY := platform.Y - float64(EnemyHeight)
					c.Pos.Y = platformTopY
					c.Vel.Y = 0
					c.OnGround = true
					c.WasOnPlatform = true
				}
				onPlatform = true
				break
			}
		}

		if !onPlatform {
			// Ground collision
			if c.Pos.Y >= groundY {
				c.Pos.Y = groundY
				if c.Vel.Y > 0 {
					c.Vel.Y = 0
				}
				c.OnGround = true
			} else {
				c.OnGround = false
			}
		}

		// Keep corpse in screen bounds
		if c.Pos.X < 0 {
			c.Pos.X = 0
			c.MoveDir = 1
		}
		if c.Pos.X > float64(g.width-EnemyWidth) {
			c.Pos.X = float64(g.width-EnemyWidth)
			c.MoveDir = -1
		}

		// Emit blood particles periodically while moving
		if now.Sub(c.LastBloodEmit) >= 80*time.Millisecond {
			// Create a temporary death particle to base emission on (add vertical spray)
			tmp := DeathParticle{
				Pos:    Vec2{X: c.Pos.X + 1.0, Y: c.Pos.Y + 1.0},
				Vel:    Vec2{X: float64(c.MoveDir) * speed, Y: -10.0 - rand.Float64()*20.0},
				EnemyID: c.EnemyID,
			}
			// Intensity based on horizontal speed but increased for visceral effect
			intensity := math.Min((math.Abs(float64(c.MoveDir))*speed/80.0)*1.4+0.15, 1.0)
			g.emitBloodFromParticle(&tmp, intensity, false)
			c.LastBloodEmit = now
		}

		// Check expiration
		if now.After(c.EndTime) {
			// Break corpse apart into regular death particles
			g.createDeathParticlesAt(c.Pos, c.Facing, c.EnemyID, c.WasOnPlatform)
			c.Active = false
		}
	}
}

func (g *Game) isTileUnderPlatform(x int) bool {
	// Check if a ground tile at x is directly under any platform
	for _, platform := range g.platforms {
		startX := int(platform.X)
		endX := int(platform.X + platform.Width)
		if x >= startX && x < endX {
			return true
		}
	}
	return false
}

func (g *Game) markGroundRed(tileX int, enemyID int, wasOnPlatform bool) {
	// Mark only the exact tile that was touched
	// Skip if particle was on platform AND tile is under a platform
	if tileX >= 0 && tileX < g.width {
		if !wasOnPlatform || !g.isTileUnderPlatform(tileX) {
			g.redGroundTiles[tileX] = enemyID
		}
	}
}

func (g *Game) markPlatformRed(platformIndex int, tileX int, enemyID int) {
	// Mark only the exact platform tile that was touched (use platform index * 10000 + x to create unique key)
	baseKey := platformIndex * 10000
	key := baseKey + tileX
	g.redPlatformTiles[key] = enemyID
}

// emitBloodFromParticle emits blood particles from a death particle
// intensity: 0.0-1.0, controls how many particles (0.0 = few, 1.0 = many)
// impact: true if this is an impact (ground hit), false for continuous emission
func (g *Game) emitBloodFromParticle(p *DeathParticle, intensity float64, impact bool) {
	// Calculate number of particles based on intensity
	// Impact: 3-6 particles, Continuous: 1-2 particles
	var numBlood int
	if impact {
		numBlood = 3 + rand.Intn(4) // 3-6 particles for impacts
		// Scale by intensity
		numBlood = int(float64(numBlood) * (0.5 + intensity*0.5))
	} else {
		numBlood = 1 + rand.Intn(2) // 1-2 particles for continuous
		// Scale by intensity
		if intensity > 0.5 {
			numBlood = 1 + rand.Intn(2) // Keep it low for continuous
		}
	}

	for i := 0; i < numBlood; i++ {
		// Realistic blood particle physics
		// Blood should spray in the direction of movement with some randomness
		var angle float64
		var speed float64

		if impact {
			// Impact: particles spray outward in all directions, biased by impact velocity
			impactAngle := math.Atan2(p.Vel.Y, p.Vel.X)
			// Add randomness around the impact direction
			angle = impactAngle + (rand.Float64()-0.5)*math.Pi*0.8 // ±72 degrees from impact direction
			// Impact speed based on how fast the piece was moving
			impactSpeed := math.Sqrt(p.Vel.X*p.Vel.X + p.Vel.Y*p.Vel.Y)
			speed = 20.0 + impactSpeed*0.3 + rand.Float64()*30.0 // 20-80+ pixels/sec
		} else {
			// Continuous: particles follow the piece's movement with trailing effect
			if math.Abs(p.Vel.X) > 0.1 || math.Abs(p.Vel.Y) > 0.1 {
				// Bias in opposite direction of movement (trailing effect)
				movementAngle := math.Atan2(p.Vel.Y, p.Vel.X)
				// Trail behind the piece
				angle = movementAngle + math.Pi + (rand.Float64()-0.5)*math.Pi*0.6 // Behind ±54 degrees
			} else {
				// Random if not moving much
				angle = rand.Float64() * 2 * math.Pi
			}
			// Continuous emission: slower particles
			speed = 10.0 + rand.Float64()*25.0 // 10-35 pixels/sec
		}

		// Add some upward bias for realistic blood spray
		velX := math.Cos(angle) * speed
		velY := math.Sin(angle)*speed - 5.0 - rand.Float64()*10.0 // Slight downward bias

		// Lifetime: shorter for continuous, longer for impacts
		var lifetime float64
		if impact {
			lifetime = 0.4 + rand.Float64()*0.6 // 0.4-1.0 seconds for impacts
		} else {
			lifetime = 0.3 + rand.Float64()*0.5 // 0.3-0.8 seconds for continuous
		}

		char := '.' // Use only '.' for smaller particles

		// Position: slight offset from particle position
		offsetX := (rand.Float64() - 0.5) * 2.0
		offsetY := (rand.Float64() - 0.5) * 2.0

		g.bloodParticles = append(g.bloodParticles, BloodParticle{
			Pos:      Vec2{X: p.Pos.X + offsetX, Y: p.Pos.Y + offsetY},
			Vel:      Vec2{X: velX, Y: velY},
			Char:     char,
			Active:   true,
			Lifetime: lifetime,
			EnemyID:  p.EnemyID,
		})
	}
}

func (g *Game) updateDeathParticles(deltaTime float64) {
	// Update any mobile corpses first so they can squirt blood and later break apart
	g.updateCorpses(deltaTime)
	gravity := 300.0                  // pixels per second squared (increased to ensure falling)
	bounceDamping := 0.4              // Velocity reduction on bounce (reduced to make bounces weaker)
	groundY := float64(g.groundY - 1) // Rest one row above ground

	// When game over, remove all non-player particles (EnemyID != 0)
	if g.gameOver {
		active := g.deathParticles[:0]
		for i := range g.deathParticles {
			// Keep only player particles (EnemyID == 0)
			if g.deathParticles[i].EnemyID == 0 {
				active = append(active, g.deathParticles[i])
			}
		}
		g.deathParticles = active
	}

	for i := range g.deathParticles {
		if !g.deathParticles[i].Active {
			continue
		}

		p := &g.deathParticles[i]

		// Check if particle is in the air (not on ground and moving)
		isInAir := !p.OnGround && (math.Abs(p.Vel.X) > 0.1 || math.Abs(p.Vel.Y) > 0.1)

		// Emit blood particles continuously from pieces in the air (following them)
		if isInAir {
			now := time.Now()
			// Emit every 50-150ms while in the air (more frequent for faster pieces)
			speed := math.Sqrt(p.Vel.X*p.Vel.X + p.Vel.Y*p.Vel.Y)
			emitInterval := 150*time.Millisecond - time.Duration(speed*0.8)*time.Millisecond
			if emitInterval < 30*time.Millisecond {
				emitInterval = 30 * time.Millisecond
			}
			if now.Sub(p.LastBloodEmit) >= emitInterval {
				// Intensity based on speed (faster = more blood)
				intensity := math.Min(speed/100.0, 0.7) // Max 0.7 for continuous
				g.emitBloodFromParticle(p, intensity, false)
				p.LastBloodEmit = now
			}
		}

		// Apply gravity (positive Y is downward in screen coordinates)
		// Always apply gravity unless particle is settled on ground with no velocity
		if !(p.OnGround && p.Vel.Y == 0) {
			p.Vel.Y += gravity * deltaTime
		}

		// Update position
		p.Pos.X += p.Vel.X * deltaTime
		p.Pos.Y += p.Vel.Y * deltaTime

		// Check platform collisions first
		onPlatform := false
		for _, platform := range g.platforms {
			// Check if particle is on top of platform (either falling onto it or already settled)
			particleBottomY := p.Pos.Y + 1.0
			isOnPlatform := p.Pos.X < platform.X+platform.Width &&
				p.Pos.X+1.0 > platform.X &&
				particleBottomY >= platform.Y &&
				particleBottomY <= platform.Y+platform.Height+1.0 // Allow small tolerance

			if isOnPlatform {
				// Particle is on or above platform
				// If falling down onto platform, bounce or settle
				if p.Vel.Y > 0 && p.Pos.Y < platform.Y {
					platformTopY := platform.Y - 1.0
					p.Pos.Y = platformTopY

					// Bounce or settle on platform
					if p.Bounces < 2 && p.Vel.Y > 20.0 {
						// Bounce from platform
						p.Bounces++
						p.Vel.Y = -p.Vel.Y * bounceDamping
						p.Vel.X *= 0.8 // Friction
						p.OnGround = false
						p.BouncedFromPlatform = true // Mark that it bounced from platform

						// Emit blood particles when bouncing (impact)
						impactSpeed := math.Abs(p.Vel.Y)
						intensity := math.Min(impactSpeed/60.0, 1.0)
						g.emitBloodFromParticle(p, intensity, true)
					} else {
						// Settle on platform
						p.Vel.Y = 0
						if !p.OnGround {
							p.OnGround = true
							p.GroundTime = time.Now()
							p.Vel.X = 0
							p.WasOnPlatform = true // Mark that particle is on platform
						}
					}
				} else if p.Vel.Y == 0 && p.Pos.Y <= platform.Y {
					// Already settled on platform - keep it there
					platformTopY := platform.Y - 1.0
					if p.Pos.Y != platformTopY {
						p.Pos.Y = platformTopY
					}
					if !p.OnGround {
						p.OnGround = true
						if p.GroundTime.IsZero() {
							p.GroundTime = time.Now()
						}
						p.WasOnPlatform = true
					}
				}
				onPlatform = true
				break
			}
		}

		// Check if particles on platforms should disappear (same as ground particles)
		if onPlatform && p.OnGround && p.Vel.Y == 0 {
			// Not rolling - stop all movement (rolling is handled separately below)
			if !(p.IsHead && p.IsRolling && p.RollDistance > 0) {
				p.Vel.X = 0
			}

			// Check if 3 seconds have passed (only if not rolling and not player pieces)
			// Player pieces (EnemyID 0) never disappear until reset
			if p.EnemyID != 0 && !(p.IsHead && p.IsRolling && p.RollDistance > 0) && time.Since(p.GroundTime) >= 3*time.Second {
				p.Active = false
			}
		}

		// Check ground collision only if not on a platform
		if !onPlatform && p.Pos.Y >= groundY {
			// If particle falls through, let it pass through and emit blood (only once)
			if p.FallsThrough && p.Vel.Y > 0 && p.Bounces == 0 {
				// Emit blood particles when falling through (impact)
				impactSpeed := math.Abs(p.Vel.Y)
				intensity := math.Min(impactSpeed/60.0, 1.0)
				g.emitBloodFromParticle(p, intensity, true)
				p.Bounces = 999 // Mark as having passed through to avoid multiple emissions
				// Continue falling - don't clamp position or stop, don't process normal collision
			} else if !p.FallsThrough || p.Bounces < 999 {
				// Normal behavior - bounce or settle
				p.Pos.Y = groundY // Clamp to ground

				// Check if particle hit the ground for the first time - emit particles on impact
				if !p.HasHitGround && p.Vel.Y > 0 {
					// First ground impact - emit blood particles
					impactSpeed := math.Abs(p.Vel.Y)
					intensity := math.Min(impactSpeed/60.0, 1.0)
					g.emitBloodFromParticle(p, intensity, true)
					p.HasHitGround = true
					p.LastBloodEmit = time.Now()
				}

				if p.Vel.Y > 0 {
					// Moving downward - check if we should bounce
					if p.Bounces < 2 && p.Vel.Y > 20.0 {
						// Bounce - reverse and dampen velocity
						wasBouncing := p.Bounces > 0
						p.Bounces++
						p.Vel.Y = -p.Vel.Y * bounceDamping
						p.Vel.X *= 0.8     // Friction
						p.OnGround = false // Allow it to move up

						// If bounced from platform, make more exaggerated splat
						if p.BouncedFromPlatform {
							// More exaggerated splat - emit blood particles (impact)
							impactSpeed := math.Abs(p.Vel.Y)
							intensity := math.Min(impactSpeed/60.0, 1.0)
							g.emitBloodFromParticle(p, intensity, true)
							p.BouncedFromPlatform = false // Reset flag
						} else {
							// Normal bounce - emit blood particles (impact)
							if wasBouncing || p.Bounces == 1 {
								impactSpeed := math.Abs(p.Vel.Y)
								intensity := math.Min(impactSpeed/60.0, 1.0)
								g.emitBloodFromParticle(p, intensity, true)
							}
						}
					} else {
						// Too slow or too many bounces - settle and stop completely
						p.Vel.Y = 0
						if !p.OnGround {
							// First time settling
							// If bounced from platform, make more exaggerated splat
							if p.BouncedFromPlatform {
								// More exaggerated splat - emit blood particles (impact)
								impactSpeed := math.Abs(p.Vel.Y)
								intensity := math.Min(impactSpeed/60.0, 1.0)
								g.emitBloodFromParticle(p, intensity, true)
								p.BouncedFromPlatform = false // Reset flag
							}
							p.OnGround = true
							p.GroundTime = time.Now()

							// If this is a head piece that should roll, start rolling
							if p.IsHead && p.IsRolling && p.RollDistance > 0 {
								p.Vel.X = p.RollSpeed
							} else {
								p.Vel.X = 0 // Stop horizontal movement if not rolling
							}
						} else {
							// Already on ground - stop horizontal movement if not rolling
							if !(p.IsHead && p.IsRolling && p.RollDistance > 0) {
								p.Vel.X = 0
							}
						}
					}
				} else if p.Vel.Y == 0 {
					// Already settled on ground
					if !p.OnGround {
						p.OnGround = true
						p.GroundTime = time.Now()

						// If this is a head piece that should roll, start rolling
						if p.IsHead && p.IsRolling && p.RollDistance > 0 {
							p.Vel.X = p.RollSpeed
						}
					}

					// Not rolling - stop all movement (rolling is handled separately below)
					if !(p.IsHead && p.IsRolling && p.RollDistance > 0) {
						p.Vel.X = 0
					}

					// Check if 3 seconds have passed (only if not rolling and not player pieces)
					// Player pieces (EnemyID 0) never disappear until reset
					if p.EnemyID != 0 && !(p.IsHead && p.IsRolling && p.RollDistance > 0) && time.Since(p.GroundTime) >= 3*time.Second {
						p.Active = false
					}
				}
				// If p.Vel.Y < 0, particle is moving upward, let it continue
			}
		}

		// Remove particles that fall off screen (including those that fall through)
		if p.Pos.Y > float64(g.height) {
			p.Active = false
		}

		// Handle rolling for head pieces on ground
		if p.OnGround && p.Vel.Y == 0 && p.IsHead && p.IsRolling && p.RollDistance > 0 {
			// Update rolling distance
			rollDelta := math.Abs(p.Vel.X) * deltaTime
			p.RollDistance -= rollDelta

			// Emit particles continuously while rolling until head comes to a stop
			// Use current velocity instead of initial roll speed
			currentSpeed := math.Abs(p.Vel.X)
			if currentSpeed > 0.1 {
				now := time.Now()
				// Emit every 80-200ms while rolling (more frequent when faster)
				emitInterval := 200*time.Millisecond - time.Duration(currentSpeed*1.5)*time.Millisecond
				if emitInterval < 50*time.Millisecond {
					emitInterval = 50 * time.Millisecond
				}
				if now.Sub(p.LastBloodEmit) >= emitInterval {
					// Intensity based on rolling speed
					intensity := math.Min(currentSpeed/60.0, 0.6) // Max 0.6 for rolling
					g.emitBloodFromParticle(p, intensity, false)
					p.LastBloodEmit = now
				}
			}

			// Apply friction to rolling
			p.Vel.X *= 0.95

			// Stop rolling when distance is exhausted or speed is too low
			if p.RollDistance <= 0 || math.Abs(p.Vel.X) < 0.5 {
				p.Vel.X = 0
				p.IsRolling = false
				p.RollDistance = 0
				// Reset ground time for the 3-second timer (only for non-player pieces)
				if p.EnemyID != 0 {
					p.GroundTime = time.Now()
				}
			}
		} else if p.OnGround && p.Vel.Y == 0 && !(p.IsHead && p.IsRolling && p.RollDistance > 0) {
			// If settled on ground and not rolling, ensure no movement
			p.Vel.X = 0
			p.Vel.Y = 0
		}

		// Remove if off screen
		if p.Pos.X < -10 || p.Pos.X > float64(g.width)+10 {
			p.Active = false
		}
	}

	// Clean up inactive particles
	active := g.deathParticles[:0]
	for i := range g.deathParticles {
		if g.deathParticles[i].Active {
			active = append(active, g.deathParticles[i])
		}
	}
	g.deathParticles = active
}

func (g *Game) updateBloodParticles(deltaTime float64) {
	gravity := 300.0 // pixels per second squared
	groundY := float64(g.groundY)

	// When game over, remove all enemy blood particles but keep player blood particles
	if g.gameOver {
		active := g.bloodParticles[:0]
		for i := range g.bloodParticles {
			// Keep only player blood particles (EnemyID == 0)
			if g.bloodParticles[i].EnemyID == 0 {
				active = append(active, g.bloodParticles[i])
			}
		}
		g.bloodParticles = active
	}

	for i := range g.bloodParticles {
		if !g.bloodParticles[i].Active {
			continue
		}

		p := &g.bloodParticles[i]

		// Apply gravity
		p.Vel.Y += gravity * deltaTime

		// Update position
		p.Pos.X += p.Vel.X * deltaTime
		p.Pos.Y += p.Vel.Y * deltaTime

		// Check platform collisions first - blood particles should be blocked by platforms
		hitPlatform := false
		for platformIndex, platform := range g.platforms {
			// Check if blood particle is colliding with platform
			if p.Pos.X < platform.X+platform.Width &&
				p.Pos.X+1.0 > platform.X &&
				p.Pos.Y < platform.Y+platform.Height &&
				p.Pos.Y+1.0 > platform.Y {
				// Blood particle hit platform - mark platform and remove particle
				tileX := int(p.Pos.X)
				if tileX >= 0 && tileX < g.width {
					g.markPlatformRed(platformIndex, tileX, p.EnemyID)
				}
				p.Active = false
				hitPlatform = true
				break
			}
		}

		// Only check ground collision if didn't hit platform
		if !hitPlatform && p.Pos.Y >= groundY {
			tileX := int(p.Pos.X)
			if tileX >= 0 && tileX < g.width {
				g.markGroundRed(tileX, p.EnemyID, false) // Blood particles never come from platforms
			}
		}

		// Update lifetime
		p.Lifetime -= deltaTime
		if p.Lifetime <= 0 {
			p.Active = false
		}

		// Remove if off screen or below ground
		if p.Pos.X < -10 || p.Pos.X > float64(g.width)+10 || p.Pos.Y > float64(g.height) {
			p.Active = false
		}
	}

	// Clean up inactive particles and check if we need to clear red tiles
	active := g.bloodParticles[:0]
	for i := range g.bloodParticles {
		if g.bloodParticles[i].Active {
			active = append(active, g.bloodParticles[i])
		}
	}
	g.bloodParticles = active
}

func (g *Game) checkAndClearRedTiles() {
	// Check if any enemy IDs have no active particles, and clear their tiles
	activeEnemyIDs := make(map[int]bool)

	// Collect all active enemy IDs from death particles
	for i := range g.deathParticles {
		if g.deathParticles[i].Active {
			activeEnemyIDs[g.deathParticles[i].EnemyID] = true
		}
	}

	// Collect all active enemy IDs from blood particles
	for i := range g.bloodParticles {
		if g.bloodParticles[i].Active {
			activeEnemyIDs[g.bloodParticles[i].EnemyID] = true
		}
	}

	// Find enemy IDs that have red ground tiles but no active particles
	tilesToClear := make(map[int]bool)
	for x, enemyID := range g.redGroundTiles {
		if !activeEnemyIDs[enemyID] {
			tilesToClear[x] = true
		}
	}

	// Clear ground tiles for enemies that are gone
	for x := range tilesToClear {
		delete(g.redGroundTiles, x)
	}

	// Find enemy IDs that have red platform tiles but no active particles
	platformTilesToClear := make(map[int]bool)
	for key, enemyID := range g.redPlatformTiles {
		if !activeEnemyIDs[enemyID] {
			platformTilesToClear[key] = true
		}
	}

	// Clear platform tiles for enemies that are gone
	for key := range platformTilesToClear {
		delete(g.redPlatformTiles, key)
	}
}

func (g *Game) drawDeathParticles() {
	now := time.Now()

	for i := range g.deathParticles {
		p := &g.deathParticles[i]
		if !p.Active {
			continue
		}

		x := int(p.Pos.X)
		y := int(p.Pos.Y)

		// Flash before disappearing (last 0.5 seconds) - but not for player pieces
		if p.EnemyID != 0 && p.OnGround && time.Since(p.GroundTime) >= 2500*time.Millisecond {
			// Flash every 100ms (only for enemy pieces, not player)
			if (now.UnixNano()/int64(100*time.Millisecond))%2 == 0 {
				continue // Skip rendering this frame
			}
		}

		// Use appropriate color: blood color if marked, yellow if from tough enemy, blue if from player, otherwise light gray
		var style tcell.Style
		if p.IsRed {
			// Use blood color mode for this piece
			if g.bloodColorMode == 3 {
				// Blood is off, use default color based on enemy type
				if p.EnemyID == 0 {
					style = tcell.StyleDefault.Foreground(tcell.ColorBlue)
				} else {
					style = tcell.StyleDefault.Foreground(tcell.ColorLightGray)
				}
			} else {
				// Apply blood color mode
				switch g.bloodColorMode {
				case 0: // Red
					style = tcell.StyleDefault.Foreground(tcell.ColorRed)
				case 1: // Green
					style = tcell.StyleDefault.Foreground(tcell.ColorGreen)
				case 2: // Rainbow (deterministic color based on position)
					colors := []tcell.Color{
						tcell.ColorRed,
						tcell.ColorYellow,
						tcell.ColorGreen,
						tcell.ColorBlue,
						tcell.ColorLightGray,
						tcell.ColorWhite,
					}
					// Use position to determine color (no time component)
					colorIndex := (x + y) % len(colors)
					style = tcell.StyleDefault.Foreground(colors[colorIndex])
				default:
					style = tcell.StyleDefault.Foreground(tcell.ColorRed)
				}
			}
		} else if p.EnemyID == 0 {
			// Player death particles (EnemyID 0) are blue
			style = tcell.StyleDefault.Foreground(tcell.ColorBlue)
		} else {
			// Enemy death particles are light gray
			style = tcell.StyleDefault.Foreground(tcell.ColorLightGray)
		}
		g.screen.SetContent(x, y, p.Char, nil, style)
	}
}

func (g *Game) drawBloodParticles() {
	if g.bloodColorMode == 3 {
		// Blood is off, don't render
		return
	}

	for i := range g.bloodParticles {
		p := &g.bloodParticles[i]
		if !p.Active {
			continue
		}

		x := int(p.Pos.X)
		y := int(p.Pos.Y)

		// Only render if on screen
		if x >= 0 && x < g.width && y >= 0 && y < g.height {
			var style tcell.Style

			switch g.bloodColorMode {
			case 0: // Red
				style = tcell.StyleDefault.Foreground(tcell.ColorRed)
			case 1: // Green
				style = tcell.StyleDefault.Foreground(tcell.ColorGreen)
			case 2: // Rainbow (deterministic color based on position)
				// Use position to create rainbow effect (no time component)
				colors := []tcell.Color{
					tcell.ColorRed,
					tcell.ColorYellow,
					tcell.ColorGreen,
					tcell.ColorBlue,
					tcell.ColorLightGray,
					tcell.ColorWhite,
				}
				colorIndex := (x + y) % len(colors)
				style = tcell.StyleDefault.Foreground(colors[colorIndex])
			default:
				style = tcell.StyleDefault.Foreground(tcell.ColorRed)
			}

			g.screen.SetContent(x, y, p.Char, nil, style)
		}
	}
}

func (g *Game) checkCollisions() {
	// Check player-enemy collisions
	for i := range g.enemies {
		if !g.enemies[i].Active {
			continue
		}

		// Simple bounding box collision
		if g.player.Pos.X < g.enemies[i].Pos.X+float64(g.enemies[i].Width) &&
			g.player.Pos.X+float64(g.player.Width) > g.enemies[i].Pos.X &&
			g.player.Pos.Y < g.enemies[i].Pos.Y+float64(g.enemies[i].Height) &&
			g.player.Pos.Y+float64(g.player.Height) > g.enemies[i].Pos.Y {
			// Player hit! Create death particles and set game over
			g.createPlayerDeathParticles()
			g.gameOver = true
			return
		}
	}

	// Check enemy projectile-player collisions
	for i := range g.projectiles {
		if !g.projectiles[i].Active || !g.projectiles[i].IsEnemy {
			continue
		}

		projX := g.projectiles[i].Pos.X
		projY := g.projectiles[i].Pos.Y
		projW := 1.0
		projH := 1.0

		// Check collision with player
		if projX < g.player.Pos.X+float64(g.player.Width) &&
			projX+projW > g.player.Pos.X &&
			projY < g.player.Pos.Y+float64(g.player.Height) &&
			projY+projH > g.player.Pos.Y {
			// Hit player! Create death particles and set game over
			g.createPlayerDeathParticles()
			g.gameOver = true
			return
		}
	}

	// Check player projectile-enemy collisions (only player projectiles)
	for i := range g.projectiles {
		if !g.projectiles[i].Active || g.projectiles[i].IsEnemy {
			continue // Skip enemy projectiles
		}

		for j := range g.enemies {
			if !g.enemies[j].Active {
				continue
			}

			// Swept AABB collision detection to handle fast-moving projectiles
			// Check if projectile's path (from previous position to current) intersects enemy
			projPrevX := g.projectiles[i].PrevPos.X
			projPrevY := g.projectiles[i].PrevPos.Y
			projCurrX := g.projectiles[i].Pos.X
			projCurrY := g.projectiles[i].Pos.Y
			projW := 1.0
			projH := 1.0

			enemyX := g.enemies[j].Pos.X
			enemyY := g.enemies[j].Pos.Y
			enemyW := float64(g.enemies[j].Width)
			enemyH := float64(g.enemies[j].Height)

			// Check current position
			hit := false
			if projCurrX < enemyX+enemyW &&
				projCurrX+projW > enemyX &&
				projCurrY < enemyY+enemyH &&
				projCurrY+projH > enemyY {
				hit = true
			}

			// Check previous position (in case we moved through it)
			if !hit && projPrevX < enemyX+enemyW &&
				projPrevX+projW > enemyX &&
				projPrevY < enemyY+enemyH &&
				projPrevY+projH > enemyY {
				hit = true
			}

			// Check intermediate positions along the path (swept collision)
			if !hit {
				// Sample points along the path
				dx := projCurrX - projPrevX
				dy := projCurrY - projPrevY
				steps := 5 // Check 5 points along the path
				for k := 1; k < steps; k++ {
					t := float64(k) / float64(steps)
					checkX := projPrevX + dx*t
					checkY := projPrevY + dy*t

					if checkX < enemyX+enemyW &&
						checkX+projW > enemyX &&
						checkY < enemyY+enemyH &&
						checkY+projH > enemyY {
						hit = true
						break
					}
				}
			}

			if hit {
				// Hit! Enemy killed (all enemies have 1 health)
				g.projectiles[i].Active = false
				g.createDeathParticles(&g.enemies[j])
				g.enemies[j].Active = false
				g.score += 10
				g.enemiesDefeated++
				break // Projectile can only hit one enemy
			}
		}
	}
}

func (g *Game) handleInput(ev *tcell.EventKey) {
	if g.inMenu {
		switch ev.Key() {
		case tcell.KeyRune:
			if ev.Rune() == ' ' {
				// Start game - reset everything
				// Clear all menu demo entities
				g.projectiles = make([]Projectile, 0)
				g.enemies = make([]Enemy, 0)
				g.deathParticles = make([]DeathParticle, 0)
				g.bloodParticles = make([]BloodParticle, 0)
				g.redGroundTiles = make(map[int]int)
				g.redPlatformTiles = make(map[int]int)

				// Reset player to starting position
				g.player = Player{
					Pos:              Vec2{X: float64(g.width / 2), Y: float64(g.groundY - PlayerHeight)},
					Vel:              Vec2{X: 0, Y: 0},
					Facing:           1,
					MoveDir:          0,
					Width:            PlayerWidth,
					Height:           PlayerHeight,
					OnGround:         true,
					OnPlatform:       false,
					LastOnGroundTime: time.Now(),
				}

				// Reset game state
				g.score = 0
				g.enemiesDefeated = 0
				g.enemySpawnCounter = 0
				g.gameOver = false
				g.nextEnemyID = 1
				g.lastShot = time.Time{}
				g.keys = make(map[tcell.Key]time.Time)

				// Recreate platforms for the new game
				platforms := make([]Platform, 0)
				numPlatforms := 3 + rand.Intn(3) // 3-5 platforms
				platformSpacing := float64(g.width) / float64(numPlatforms+1)
				groundLevel := float64(g.groundY)
				for i := 0; i < numPlatforms; i++ {
					platformX := platformSpacing * float64(i+1)
					heightAboveGround := 4.0 + rand.Float64()*8.0      // 4-12 pixels above ground
					platformY := groundLevel - heightAboveGround - 1.0 // -1 to account for platform height
					platformWidth := 12.0 + rand.Float64()*16.0        // 12-28 wide - wider platforms
					platforms = append(platforms, Platform{
						X:      platformX - platformWidth/2,
						Y:      platformY,
						Width:  platformWidth,
						Height: 1.0,
					})
				}
				g.platforms = platforms

				// Start game
				g.inMenu = false
			}
		case tcell.KeyTab:
			// Toggle blood color mode
			g.bloodColorMode = (g.bloodColorMode + 1) % 4
		case tcell.KeyEscape:
			// Exit handled by main loop
		}
		return
	}

	if g.gameOver {
		switch ev.Key() {
		case tcell.KeyEnter:
			// Restart game - go back to menu
			// Recreate platforms on restart
			platforms := make([]Platform, 0)
			numPlatforms := 3 + rand.Intn(3) // 3-5 platforms
			platformSpacing := float64(g.width) / float64(numPlatforms+1)
			groundLevel := float64(g.groundY)
			for i := 0; i < numPlatforms; i++ {
				platformX := platformSpacing * float64(i+1)
				// Platforms at different heights: 4-12 pixels above ground (reachable with jump)
				heightAboveGround := 4.0 + rand.Float64()*8.0      // 4-12 pixels above ground
				platformY := groundLevel - heightAboveGround - 1.0 // -1 to account for platform height
				platformWidth := 12.0 + rand.Float64()*16.0        // 12-28 wide - wider platforms
				platforms = append(platforms, Platform{
					X:      platformX - platformWidth/2,
					Y:      platformY,
					Width:  platformWidth,
					Height: 1.0,
				})
			}

			g.player = Player{
				Pos:              Vec2{X: float64(g.width / 2), Y: float64(g.groundY - PlayerHeight)},
				Vel:              Vec2{X: 0, Y: 0},
				Facing:           1,
				MoveDir:          0,
				Width:            PlayerWidth,
				Height:           PlayerHeight,
				OnGround:         true,
				OnPlatform:       false,
				LastOnGroundTime: time.Now(),
			}
			g.projectiles = make([]Projectile, 0)
			g.enemies = make([]Enemy, 0)
			g.platforms = platforms
			// Clear all particles on restart
			g.deathParticles = make([]DeathParticle, 0)
			g.bloodParticles = make([]BloodParticle, 0)
			g.redGroundTiles = make(map[int]int)
			g.redPlatformTiles = make(map[int]int)
			g.score = 0
			g.enemiesDefeated = 0
			g.enemySpawnCounter = 0
			g.gameOver = false
			g.inMenu = true
			g.keys = make(map[tcell.Key]time.Time)
		case tcell.KeyEscape:
			// Exit handled by main loop
		}
		return
	}

	// Handle TAB to toggle blood color even during gameplay
	if ev.Key() == tcell.KeyTab {
		g.bloodColorMode = (g.bloodColorMode + 1) % 4
		return
	}

	// Track key states for smooth movement
	switch ev.Key() {
	case tcell.KeyLeft:
		g.keys[tcell.KeyLeft] = time.Now()
	case tcell.KeyRight:
		g.keys[tcell.KeyRight] = time.Now()
	case tcell.KeyUp:
		g.keys[tcell.KeyUp] = time.Now()
	case tcell.KeyRune:
		if ev.Rune() == ' ' {
			// Fire projectile (with cooldown to prevent spam)
			now := time.Now()
			if now.Sub(g.lastShot) > 200*time.Millisecond {
				p := Projectile{
					Pos:     Vec2{X: g.player.Pos.X + float64(g.player.Width/2), Y: g.player.Pos.Y + float64(g.player.Height/2)},
					PrevPos: Vec2{X: g.player.Pos.X + float64(g.player.Width/2), Y: g.player.Pos.Y + float64(g.player.Height/2)},
					Dir:     g.player.Facing,
					Active:  true,
					Frame:   0,
					IsEnemy: false,
				}
				g.projectiles = append(g.projectiles, p)
				g.lastShot = now
			}
		}
	}
}

func (g *Game) updateMenu(deltaTime float64) {
	// Update menu demo - player fires projectiles and enemies spawn
	now := time.Now()

	// Position menu player in center of screen
	if g.player.Pos.X == 0 && g.player.Pos.Y == 0 {
		g.player.Pos.X = float64(g.width / 2)
		g.player.Pos.Y = float64(g.groundY - PlayerHeight)
		g.player.Facing = 1
	}

	// Menu player fires projectiles periodically (every 0.8 seconds)
	if now.Sub(g.menuLastShot) > 800*time.Millisecond {
		// Fire in random direction
		dir := 1
		if rand.Float64() < 0.5 {
			dir = -1
		}
		g.player.Facing = dir

		p := Projectile{
			Pos:     Vec2{X: g.player.Pos.X + float64(g.player.Width/2), Y: g.player.Pos.Y + float64(g.player.Height/2)},
			PrevPos: Vec2{X: g.player.Pos.X + float64(g.player.Width/2), Y: g.player.Pos.Y + float64(g.player.Height/2)},
			Dir:     dir,
			Active:  true,
			Frame:   0,
			IsEnemy: false,
		}
		g.projectiles = append(g.projectiles, p)
		g.menuLastShot = now
	}

	// Spawn enemies occasionally (every 1.5-3 seconds)
	if now.Sub(g.menuLastEnemySpawn) > time.Duration(1500+rand.Intn(1500))*time.Millisecond {
		var e Enemy
		canShoot := false // Menu enemies don't shoot

		if rand.Float64() < 0.5 {
			// Spawn from left
			e = Enemy{
				Pos:           Vec2{X: -float64(EnemyWidth), Y: float64(g.groundY - EnemyHeight)},
				Vel:           Vec2{X: 0, Y: 0},
				Facing:        1,
				Width:         EnemyWidth,
				Height:        EnemyHeight,
				Active:        true,
				CanShoot:      canShoot,
				LastShot:      time.Time{},
				NextShotDelay: 0,
				OnGround:      true,
				JumpCooldown:  time.Time{},
			}
		} else {
			// Spawn from right
			e = Enemy{
				Pos:           Vec2{X: float64(g.width), Y: float64(g.groundY - EnemyHeight)},
				Vel:           Vec2{X: 0, Y: 0},
				Facing:        -1,
				Width:         EnemyWidth,
				Height:        EnemyHeight,
				Active:        true,
				CanShoot:      canShoot,
				LastShot:      time.Time{},
				NextShotDelay: 0,
				OnGround:      true,
				JumpCooldown:  time.Time{},
			}
		}
		g.enemies = append(g.enemies, e)
		g.menuLastEnemySpawn = now
	}

	// Update menu enemies (move towards center, no jumping)
	baseSpeed := 20.0
	for i := range g.enemies {
		if !g.enemies[i].Active {
			continue
		}

		// Move towards center of screen
		centerX := float64(g.width / 2)
		dx := centerX - g.enemies[i].Pos.X
		if dx > 0 {
			g.enemies[i].Facing = 1
			g.enemies[i].Pos.X += baseSpeed * deltaTime
		} else {
			g.enemies[i].Facing = -1
			g.enemies[i].Pos.X -= baseSpeed * deltaTime
		}

		// Remove enemies that go off screen or reach center
		if g.enemies[i].Pos.X < -float64(EnemyWidth) ||
			g.enemies[i].Pos.X > float64(g.width) ||
			math.Abs(g.enemies[i].Pos.X-centerX) < 5.0 {
			g.enemies[i].Active = false
		}
	}

	// Clean up inactive enemies
	active := g.enemies[:0]
	for i := range g.enemies {
		if g.enemies[i].Active {
			active = append(active, g.enemies[i])
		}
	}
	g.enemies = active

	// Update projectiles
	g.updateProjectiles(deltaTime)

	// Check collisions for menu demo (enemies hit by projectiles)
	for i := range g.projectiles {
		if !g.projectiles[i].Active || g.projectiles[i].IsEnemy {
			continue
		}

		for j := range g.enemies {
			if !g.enemies[j].Active {
				continue
			}

			projX := g.projectiles[i].Pos.X
			projY := g.projectiles[i].Pos.Y
			enemyX := g.enemies[j].Pos.X
			enemyY := g.enemies[j].Pos.Y

			if projX < enemyX+float64(g.enemies[j].Width) &&
				projX+1.0 > enemyX &&
				projY < enemyY+float64(g.enemies[j].Height) &&
				projY+1.0 > enemyY {
				// Hit! Create death particles and remove enemy
				g.createDeathParticles(&g.enemies[j])
				g.enemies[j].Active = false
				g.projectiles[i].Active = false
				break
			}
		}
	}

	// Update death particles
	g.updateDeathParticles(deltaTime)
	g.updateBloodParticles(deltaTime)
	g.checkAndClearRedTiles()
}

func (g *Game) update(deltaTime float64) {
	// Update menu demo if in menu
	if g.inMenu {
		g.updateMenu(deltaTime)
		return
	}

	if !g.gameOver {
		// Only update player when game is active
		g.updatePlayer(deltaTime)
		g.checkCollisions()
	}

	// Always update projectiles, enemies, and particles (even when game over)
	g.updateProjectiles(deltaTime)
	g.updateEnemies(deltaTime)
	g.updateDeathParticles(deltaTime)
	g.updateBloodParticles(deltaTime)
	g.checkAndClearRedTiles() // Clear red tiles for enemies that are gone
}

func (g *Game) render() {
	g.screen.Clear()

	if g.inMenu {
		// Draw game elements for menu demo
		g.drawGround()
		g.drawPlatforms()
		g.drawPlayer()

		for i := range g.projectiles {
			if g.projectiles[i].Active {
				g.drawProjectile(&g.projectiles[i])
			}
		}

		for i := range g.enemies {
			if g.enemies[i].Active {
				g.drawEnemy(&g.enemies[i])
			}
		}

		// Draw any mobile corpses (decapitated bodies)
		for i := range g.corpses {
			if g.corpses[i].Active {
				g.drawCorpse(&g.corpses[i])
			}
		}

		g.drawDeathParticles()
		g.drawBloodParticles()

		// Draw menu text on top
		g.drawMenu()
	} else {
		g.drawGround()
		g.drawPlatforms()
		g.drawScore()

		if !g.gameOver {
			g.drawPlayer()
		} else {
			g.drawGameOver()
		}

		for i := range g.projectiles {
			if g.projectiles[i].Active {
				g.drawProjectile(&g.projectiles[i])
			}
		}

		for i := range g.enemies {
			if g.enemies[i].Active {
				g.drawEnemy(&g.enemies[i])
			}
		}

		// Draw any mobile corpses (decapitated bodies)
		for i := range g.corpses {
			if g.corpses[i].Active {
				g.drawCorpse(&g.corpses[i])
			}
		}

		g.drawDeathParticles()
		g.drawBloodParticles()
	}

	g.screen.Show()
}

func (g *Game) run() {
	// Start input handling goroutine
	inputChan := make(chan tcell.Event, 10)
	go func() {
		for {
			ev := g.screen.PollEvent()
			inputChan <- ev
		}
	}()

	ticker := time.NewTicker(FrameDuration)
	defer ticker.Stop()

	for {
		// Handle timing
		now := time.Now()
		deltaTime := now.Sub(g.lastFrame).Seconds()
		g.lastFrame = now

		// Cap delta time to prevent large jumps
		if deltaTime > 0.1 {
			deltaTime = 0.1
		}

		// Handle input (non-blocking)
		select {
		case ev := <-inputChan:
			switch ev := ev.(type) {
			case *tcell.EventKey:
				if ev.Key() == tcell.KeyEscape {
					return
				}
				g.handleInput(ev)
			case *tcell.EventResize:
				g.width, g.height = g.screen.Size()
				g.groundY = g.height - 1 // Update ground position
				// Keep player in bounds after resize
				if g.player.Pos.X+float64(g.player.Width) > float64(g.width) {
					g.player.Pos.X = float64(g.width - g.player.Width)
				}
				// Update player Y position to stay on ground
				g.player.Pos.Y = float64(g.groundY - PlayerHeight)
			}
		default:
			// No input available, continue
		}

		// Clear key states if keys aren't being pressed (simple approach)
		// In a real implementation, we'd track key releases, but for simplicity
		// we'll let keys stay pressed until another key is pressed

		// Update game state
		g.update(deltaTime)

		// Render
		g.render()

		// Wait for next frame
		<-ticker.C
	}
}

func main() {
	// Initialize random seed
	rand.Seed(time.Now().UnixNano())

	// Initialize screen
	screen, err := tcell.NewScreen()
	if err != nil {
		panic(err)
	}

	if err := screen.Init(); err != nil {
		panic(err)
	}
	defer screen.Fini()

	screen.SetStyle(tcell.StyleDefault.
		Background(tcell.ColorDefault).
		Foreground(tcell.ColorWhite))
	screen.Clear()

	// Create and run game
	game := NewGame(screen)
	game.run()
}
