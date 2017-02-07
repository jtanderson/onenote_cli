package main

/* BIG TODO
*  1. Fix up state transition model
*    - channels between api fetching methods and renderer look weird
*    - E.g. need more abrupt transition to the auth state from a regular view
*    - figure out how to highlight and "click" on menu items terminal_velocity style
*  2. Shore up API key storage, security
*  3. Have a plan for cacheing vs. syncing
*  4. Clean up the module structure, separate into modules (PLEASE DEAR GOD)
 */

import (
	"encoding/base64"
	"fmt"

	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/microsoft"

	"net"

	"encoding/gob"

	"github.com/jroimartin/gocui"
)

//var l net.Listener
var user User

// ViewState represents the current layout needed
type ViewState int

const (
	// StateAuthenticate is used when the user needs to take action
	// to obtain a new or refreshed OAuth token
	StateAuthenticate = iota

	// StateNotebooks is the state showing all notebooks
	StateNotebooks
)

const (
	// URLPages stores the OneNote pages base url
	URLPages = "https://www.onenote.com/api/v1.0/me/notes/pages"

	// URLNotebooks stores the OneNote notebooks base url
	URLNotebooks = "https://www.onenote.com/api/v1.0/me/notes/notebooks"
)

var viewStateName = map[ViewState]string{
	StateAuthenticate: "authenticate",
	StateNotebooks:    "notebooks",
}

// Notebook is the datatype representing a notebook
type Notebook struct {
	Name string
}

// User is the structure holding relevant data
type User struct {
	ID               string
	Config           *oauth2.Config
	Token            *oauth2.Token
	Context          context.Context
	CurrentViewState ViewState
	Window           *gocui.Gui
	Notebooks        []Notebook
}

// Get does an http GET with the user's credentials
func (u *User) Get(url string) (*http.Response, error) {
	return u.Client().Get(url)
}

// Client returns a pointer to the http client for the user
func (u *User) Client() *http.Client {
	c := make(chan string)
	go u.Auth(c)
	<-c
	return u.Config.Client(oauth2.NoContext, u.Token)
}

// LoadNotebooks will get all the user notebooks and store them in the struct
func (u *User) LoadNotebooks(ready chan bool) {
	r, err := u.Get(URLNotebooks)
	if err != nil {
		log.Println("Getting notebooks failed")
	}
	//log.Println(r)
	defer r.Body.Close()

	notebooks, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(err)
	}

	// resp := make(map[string]interface{})
	var resp map[string]*json.RawMessage
	json.Unmarshal(notebooks, &resp)
	json.Unmarshal(*resp["value"], &u.Notebooks)
	log.Println(u.Notebooks)
	ready <- true
}

// Auth makes sure the authentication is up-to-date
func (u *User) Auth(link chan string) {
	if !u.Token.Valid() {
		// TODO: skip the user click action if we don't need to)
		log.Println("Token detected as invalid")

		r, _ := http.Get("https://www.onenote.com/api/v1.0/me/notes/pages")

		if r.StatusCode != 200 {
			log.Println("Pages probe failed. Need to Auth")
			state := randToken()

			l, err := net.Listen("tcp", ":12345")
			http.HandleFunc("/auth", makeHandlerFunc(handler, l))
			if err != nil {
				log.Fatalln(err)
			}
			go http.Serve(l, nil)
			log.Println("Now awaiting authentication redirect...")
			link <- u.Config.AuthCodeURL(state)
		} else {
			log.Println("Pages probe returned status 200")
			u.SetViewState(StateNotebooks)
			link <- "none"
		}
	} else {
		log.Println("Token is still valid")
		u.SetViewState(StateNotebooks)
		link <- "none"
	}
}

// LogOut requests a logout via the api
func (u *User) LogOut() {
	// TODO: set RedirectURL to urn:ietf:wg:oauth:2.0:oob
	_, err := u.Get("https://login.live.com/oauth20_logout.srf?client_id=" + u.Config.ClientID)
	if err != nil {
		fmt.Println("Error!")
		return
	}
	fmt.Println("Signed out successfully...")
}

func init() {
	// Load the config data
	conf := &oauth2.Config{
		ClientID:     "", // Should be set inside ./creds.json
		ClientSecret: "", // Should be set inside ./creds.json
		Scopes: []string{
			"openid",
			"office.onenote_update",
		},
		Endpoint: microsoft.LiveConnectEndpoint,
	}

	file, err := ioutil.ReadFile("./creds.json")
	if err != nil {
		log.Printf("File error: %v\n", err)
		os.Exit(1)
	}

	json.Unmarshal(file, &conf)

	user.Config = conf
}

// Load grabs the gob data from disk if it exists
func (u *User) Load() {
	userGob, err := os.Open("user.gob")
	if err != nil {
		log.Fatalln("Cannot open user gob")
	}
	decoder := gob.NewDecoder(userGob)
	var tok oauth2.Token
	err = decoder.Decode(&tok)
	if err != nil {
		log.Println(err)
	}
	log.Println("Loading token")
	//log.Println(tok)
	user.Token = &tok
	userGob.Close()
}

// Save writes the user data to disk to preserve between sessions
func (u *User) Save() {
	os.Remove("user.gob")
	userGob, err := os.OpenFile("user.gob", os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		log.Fatalln("Cannot open user gob")
	}
	encoder := gob.NewEncoder(userGob)
	tok := user.Token
	//log.Println("Saving user token")
	//log.Println(tok)
	err = encoder.Encode(tok)
	if err != nil {
		log.Println(err)
	}
	userGob.Close()
}

func main() {
	// Set us up some basic logging
	f, err := os.OpenFile("log.txt", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalln("error opening file: %v", err)
	}
	defer f.Close()
	log.SetOutput(f)

	user.Load()

	log.Println("Setting up gui")
	user.Window, err = gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		log.Fatalln(err)
	}
	defer user.Window.Close()

	user.Window.SetManagerFunc(layout)

	if err := user.Window.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		log.Fatalln(err)
	}

	if err := user.Window.MainLoop(); err != nil && err != gocui.ErrQuit {
		log.Panicln(err)
	}

	log.Println("===========================================")
}

func makeHandlerFunc(fn http.HandlerFunc, l net.Listener) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fn(w, r)
		l.Close()
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	//TODO: validate FormValue("state")
	tok, err := user.Config.Exchange(oauth2.NoContext, r.FormValue("code"))
	if err != nil {
		log.Println("Token exchange error.")
		log.Println(err)
		io.WriteString(w, "Sorry. There has been an error: ")
	} else {
		log.Println("User token successfully received")
		io.WriteString(w, "You have been successfully authenticated!")
		user.Token = tok
	}

	//log.Println(user)
	user.Save()
	user.SetViewState(StateNotebooks)
}

func randToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// SetViewState changes the state and forces a re-draw
func (u *User) SetViewState(state ViewState) {
	u.CurrentViewState = state
	u.Window.Execute(func(g *gocui.Gui) error { return nil })
}

func layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	switch user.CurrentViewState {
	case StateAuthenticate:
		if v, err := g.SetView("signin_link", 0, maxY/2-10, maxX-1, maxY/2+10); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.Wrap = true
			link := make(chan string)
			fmt.Fprintln(v, "Getting authentication link...")
			go user.Auth(link)
			lnk := <-link
			fmt.Fprintln(v, lnk)
			log.Println("Authentication link:\n" + lnk)
		}
		break
	case StateNotebooks:
		if v, err := g.SetView("notebooks", 0, 0, maxX-1, maxY-1); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			ready := make(chan bool)
			fmt.Fprintln(v, "Loading notebooks...")
			go user.LoadNotebooks(ready)
			<-ready
			v.Clear()
			for _, n := range user.Notebooks {
				fmt.Fprintln(v, n.Name)
			}
		}
	}

	return nil
}

// GetPages set up the view of pages
func (u *User) GetPages(g *gocui.Gui) {
	pages, err := u.Get("https://www.onenote.com/api/v1.0/me/notes/pages")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(pages)
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}