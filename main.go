package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const huntChance = 0.5 		// Chance for a player to attempt a hunt
const rescueChance = 0.01 	// Chance for a player to attempt a rescue
const baseChance = 0.2 		// Chance for a player to exit/enter base

var teams []string
var hunts map[string]string

// Prometheus metrics
var (
	huntedCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "playtag_hunted_players_total",
			Help: "Total number of players hunted",
		},
		[]string{"team", "target", "hunter"},
	)

	rescuedCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "playtag_rescued_players_total",
			Help: "Total number of players rescued",
		},
		[]string{"team", "player"},
	)

	treasonCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "playtag_commited_treasons_total",
			Help: "Total number of commited",
		},
		[]string{"team", "player"},
	)

	huntedGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "playtag_hunted_players_current",
			Help: "Current number of players hunted",
		},
		[]string{"team"},
	)

	inBaseGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "playtag_in_base_players_current",
			Help: "Current number of players in their base",
		},
		[]string{"team", "player"},
	)
)



func init() {
	prometheus.MustRegister(huntedCounter)
	prometheus.MustRegister(rescuedCounter)
	prometheus.MustRegister(treasonCounter)
	prometheus.MustRegister(huntedGauge)
	prometheus.MustRegister(inBaseGauge)
}

// Player represents a player in the game.
type Player struct {
	id         int
	team       string
	hunted     bool
	inBase	   bool
	teamMates  []*Player
	notifyChan chan *Player
}


// Game represents the game state.
type Game struct {
	players    []*Player
	baseMux    sync.Mutex
	hunterBase map[string][]*Player
}

func main() {
	hunts = map[string]string {
		"fox": "chicken",
		"chicken": "snake",
		"snake": "fox",
	}

	for k := range hunts {
		teams = append(teams, k)
	}

	rand.Seed(time.Now().UnixNano())

	// Create a game with players
	game := initializeGame(120)

	// Expose Prometheus metrics
	go exposeMetrics()
	
	// Run the game
	runGame(game)

	fmt.Println("Game over!")
}

// exposeMetrics starts an HTTP server to expose Prometheus metrics.
func exposeMetrics() {
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(":8080", nil)
}

// handleInterrupt listens for the interrupt signal (Ctrl+C) and stops the game when received.
func handleInterrupt() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
}

// initializeGame creates players and assigns them to teams.
func initializeGame(numPlayers int) *Game {
	game := &Game{}
	for i := 0; i < numPlayers; i++ {
		player := &Player{
			id:         i+1,
			team:       teams[i % len(teams)],
			hunted:     false,
			inBase:		true,
			notifyChan: make(chan *Player),
		}
		inBaseGauge.WithLabelValues(player.team, strconv.Itoa(player.id)).Inc()

		game.players = append(game.players, player)
	}
	game.hunterBase = map[string][]*Player{}

	for _, team := range teams {
		game.hunterBase[team] = []*Player{}
	}

	return game
}

// runGame starts the game and handles player movements.
func runGame(game *Game) {
	for _, player := range game.players {
		go player.play(game)
	}

	// Wait for the Ctrl+C signal to stop the game
	handleInterrupt()

	fmt.Println("Game over!")
}

// play simulates a player's actions in the game.
func (p *Player) play(game *Game) {
	for {
		game.baseMux.Lock()
		if p.inBase && rand.Float64() < baseChance {
			p.inBase = false
			inBaseGauge.WithLabelValues(p.team,  strconv.Itoa(p.id)).Dec()
		}

		if !p.hunted  && !p.inBase{
			// Move to another player and try to hunt
			target := findTarget(game.players, p)

			if rand.Float64() < rescueChance { // Random chance for a player to attempt a rescue
				p.rescueTeammates(game)
			} else if rand.Float64() < baseChance { // Random chance for a player to return to base
				fmt.Printf("Player %d from team %s returns to base\n", p.id, p.team)
				p.inBase = true
				inBaseGauge.WithLabelValues(p.team,  strconv.Itoa(p.id)).Inc()
			} else if target != nil  && rand.Float64() < huntChance { // Random chance for a player to hunt another
				fmt.Printf("Player %d from team %s hunted player %d from team %s\n", p.id, p.team, target.id, target.team)
				target.hunted = true
				game.hunterBase[p.team] = append(game.hunterBase[p.team], target)
				huntedCounter.WithLabelValues(target.team, strconv.Itoa(target.id), strconv.Itoa(p.id)).Inc()
				huntedGauge.WithLabelValues(target.team).Inc()
				time.Sleep(time.Duration(rand.Intn(500)) * time.Millisecond) // Simulate time taken to catch
			} else if target == nil { // No players left to hunt
				// Commit treason and liberate the enemy
				p.commitTreason(game)
			}
		}
		game.baseMux.Unlock()

		// Simulate other actions
		time.Sleep(time.Duration(rand.Intn(500)) * time.Millisecond)
	}
}

// findTarget finds a target player to hunt based on team relationships.
func findTarget(players []*Player, currentPlayer *Player) *Player {
	for _, target := range players {
		if target.team == hunts[currentPlayer.team] && !target.hunted && !target.inBase {
			return target
		}
	}
	return nil
}

// rescueTeammates attempts to rescue all hunted teammates from enemy bases.
func (p *Player) rescueTeammates(game *Game) {
	for team, base := range game.hunterBase {
		if p.team == hunts[team] {
			for _, teammate := range base {
				fmt.Printf("Player %d rescued teammate %d from team %s\n", p.id, teammate.id, teammate.team)
				teammate.hunted = false
				rescuedCounter.WithLabelValues(p.team,  strconv.Itoa(p.id)).Inc()
				huntedGauge.WithLabelValues(p.team).Dec()
			}

			// Clear the hunter base
			game.hunterBase[team] = []*Player{}
		}
	}
}

// commitTreason attempts to free all hunted enemies from player's base.
func (p *Player) commitTreason(game *Game) {
	for _, enemy := range game.hunterBase[p.team] {
		fmt.Printf("Player %d commits treason and frees %d from team %s!!!\n", p.id, enemy.id, p.team)
		enemy.hunted = false
		rescuedCounter.WithLabelValues(enemy.team,  strconv.Itoa(enemy.id)).Inc()
		huntedGauge.WithLabelValues(enemy.team).Dec()
	}

	treasonCounter.WithLabelValues(p.team, strconv.Itoa(p.id)).Inc()
	// Clear the hunter base
	game.hunterBase[p.team] = []*Player{}
}

// removePlayerFromSlice removes a player from a slice.
func removePlayerFromSlice(players []*Player, target *Player) []*Player {
	for i, player := range players {
		if player == target {
			return append(players[:i], players[i+1:]...)
		}
	}
	return players
}
