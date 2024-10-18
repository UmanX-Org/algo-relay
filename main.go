package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/fiatjaf/khatru"
	"github.com/joho/godotenv"
	"github.com/nbd-wtf/go-nostr"
)

var ctx = context.Background()
var pool = nostr.NewSimplePool(ctx)
var repository *NostrRepository
var relays = []string{
	"wss://relay.lexingtonbitcoin.org",
	"wss://nostr.600.wtf",
	"wss://nostr.hexhex.online",
	"wss://wot.utxo.one",
	"wss://nostrelites.org",
	"wss://wot.nostr.party",
	"wss://wot.puhcho.me",
	"wss://wot.girino.org",
	"wss://relay.beeola.me",
	"wss://zap.watch",
	"wss://wot.yeghro.site",
	"wss://wot.innovativecerebrum.ai",
	"wss://wot.swarmstr.com",
	"wss://wot.azzamo.net",
	"wss://satsage.xyz",
	"wss://wot.sandwich.farm",
	"wss://wons.calva.dev",
	"wss://wot.shaving.kiwi",
	"wss://wot.tealeaf.dev",
	"wss://wot.dtonon.com",
	"wss://wot.relay.vanderwarker.family",
	"wss://wot.zacoos.com",
}

var db *sql.DB
var art = `
 █████╗ ██╗      ██████╗  ██████╗     ██████╗ ███████╗██╗      █████╗ ██╗   ██╗
██╔══██╗██║     ██╔════╝ ██╔═══██╗    ██╔══██╗██╔════╝██║     ██╔══██╗╚██╗ ██╔╝
███████║██║     ██║  ███╗██║   ██║    ██████╔╝█████╗  ██║     ███████║ ╚████╔╝ 
██╔══██║██║     ██║   ██║██║   ██║    ██╔══██╗██╔══╝  ██║     ██╔══██║  ╚██╔╝  
██║  ██║███████╗╚██████╔╝╚██████╔╝    ██║  ██║███████╗███████╗██║  ██║   ██║   
╚═╝  ╚═╝╚══════╝ ╚═════╝  ╚═════╝     ╚═╝  ╚═╝╚══════╝╚══════╝╚═╝  ╚═╝   ╚═╝   
	`

func main() {
	nostr.InfoLogger = log.New(io.Discard, "", 0)
	green := "\033[32m"
	reset := "\033[0m"
	fmt.Println(green + art + reset)

	importFlag := flag.Bool("import", false, "Run the importNotes function after initializing relays")
	flag.Parse()
	conn, err := getDBConnection()

	if err != nil {
		log.Fatalf("Error getting db connection: %v", err)
	}
	defer conn.Close()
	db = conn
	repository = NewNostrRepository(db)

	if *importFlag {
		log.Println("📦 importing notes")
		importNotes(nostr.KindTextNote)
		importNotes(nostr.KindReaction)
		importNotes(nostr.KindZap)
		log.Println("📦 done importing notes. Please restart relay")
		return
	}

	go subscribeAll()

	relay := khatru.NewRelay()
	relay.Info.Description = os.Getenv("RELAY_DESCRIPTION")
	relay.Info.Name = os.Getenv("RELAY_NAME")
	relay.Info.PubKey = os.Getenv("RELAY_PUBKEY")
	relay.Info.Software = "https://github.com/bitvora/algo-relay"
	relay.Info.Version = "0.1.0"
	relay.Info.Icon = os.Getenv("RELAY_ICON")

	relay.OnConnect = append(relay.OnConnect, func(ctx context.Context) {
		khatru.RequestAuth(ctx)
	})
	relay.RejectFilter = append(relay.RejectFilter, func(ctx context.Context, filter nostr.Filter) (bool, string) {
		authenticatedUser := khatru.GetAuthed(ctx)
		if authenticatedUser == "" {
			return true, "auth-required: this query requires you to be authenticated"
		}
		return false, ""
	})
	relay.RejectEvent = append(relay.RejectEvent, func(ctx context.Context, event *nostr.Event) (bool, string) {
		return true, "you cannot publish to this relay"
	})

	relay.QueryEvents = append(relay.QueryEvents, func(ctx context.Context, filter nostr.Filter) (chan *nostr.Event, error) {
		ch := make(chan *nostr.Event)
		copyFilter := filter
		authenticatedUser := khatru.GetAuthed(ctx)

		go func() {
			defer close(ch)

			limit := copyFilter.Limit
			if limit == 0 {
				limit = 50
			}

			log.Println("Fetching user feed for", authenticatedUser)

			events, err := GetUserFeed(ctx, authenticatedUser, limit)
			if err != nil {
				log.Println("Error fetching most reacted posts:", err)
				return
			}

			for _, event := range events {
				ch <- &event
			}
		}()

		return ch, nil
	})

	log.Println("🚀 Relay started on port 3334")
	err = http.ListenAndServe(":3334", relay)
	if err != nil {
		log.Fatal(err)
	}

}

func subscribeAll() {
	repository := NewNostrRepository(db)
	filters := nostr.Filters{{
		Kinds: []int{
			nostr.KindTextNote,
			nostr.KindReaction,
			nostr.KindZap,
		},
	}}

	for ev := range pool.SubMany(ctx, relays, filters) {
		err := repository.SaveNostrEvent(ev.Event)
		if err != nil {
			continue
		}
	}
}

func loadEnv() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
}
