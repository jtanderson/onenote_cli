package main

/* BIG TODO
*  1. Double check view state transition model for weirdness (use logs)
*  2. Shore up API key storage, security
*     - We can drop the client secret if we use the implciit MS flow,
*       unfortunately, this means we need the redirect URI to have the access
*       in a url fragment, which Go doesn't like...
*  3. Have a plan for cacheing vs. syncing
*  4. Clean up the module structure, separate into modules
*  5. Idea: don't invalidate notebooks, sections, and pages until you get higher in the tree
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
	termbox "github.com/nsf/termbox-go"

	"github.com/clbanning/mxj"
)

//var l net.Listener
var user User

// ViewState represents the current layout needed
type ViewState int

const (
	// StateStartAuthenticate is used when the user needs to take action
	// to obtain a new or refreshed OAuth token
	StateStartAuthenticate = iota

	// StateFinishAuthenticate is the end of the auth process
	StateFinishAuthenticate

	// StateLoadNotebooks is loading notebooks
	StateLoadNotebooks

	// StateViewNotebooks shows the notebooks
	StateViewNotebooks

	// StateLoadSections loads the sections
	StateLoadSections

	// StateViewSections shows the notebook sections
	StateViewSections

	// StateLoadPages loads the section pages
	StateLoadPages

	// StateViewPages shows the secion pages
	StateViewPages

	// StateLoadPage loads a section page
	StateLoadPage

	// StateViewPage shows the section page
	StateViewPage
)

const (
	// URLGETNotebooks stores the OneNote notebooks base url
	URLGETNotebooks = "https://www.onenote.com/api/v1.0/me/notes/notebooks"

	// URLGETSections stores the OneNote sections base url
	URLGETSections = "https://www.onenote.com/api/v1.0/me/notes/notebooks/%s/sections"

	// URLGETPages stores the OneNote pages base url
	URLGETPages = "https://www.onenote.com/api/v1.0/me/notes/sections/%s/pages"

	// URLGETPage stores the OneNote page base url
	URLGETPage = "https://www.onenote.com/api/v1.0/me/notes/pages/%s/content"
)

var viewStateName = map[ViewState]string{
	StateStartAuthenticate:  "startauthenticate",
	StateFinishAuthenticate: "finishauthenticate",
	StateLoadNotebooks:      "loadnotebooks",
	StateViewNotebooks:      "viewnotebooks",
	StateLoadSections:       "loadsections",
	StateViewSections:       "viewsections",
	StateLoadPages:          "loadpages",
	StateViewPages:          "viewpages",
	StateLoadPage:           "loadpage",
	StateViewPage:           "viewpage",
}

// Notebook is the datatype representing a notebook
type Notebook struct {
	Name string
	ID   string
}

// Section is a notebook section
type Section struct {
	Name string
	ID   string
}

// Page is a section page
type Page struct {
	Title      string
	ID         string
	Content    mxj.Map
	ContentURL string
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
	CurrentNotebook  Notebook
	Sections         []Section
	CurrentSection   Section
	Pages            []Page
	CurrentPage      Page
	StateData        string
}

// Get does an http GET with the user's credentials
func (u *User) Get(url string, args ...interface{}) (*http.Response, error) {
	if u.LoggedIn() {
		getURL := fmt.Sprintf(url, args...)
		log.Println("Starting get request:")
		log.Println(getURL)
		return u.Config.Client(oauth2.NoContext, u.Token).Get(getURL)
	}
	u.SetViewState(StateStartAuthenticate)
	return nil, nil
}

// LoadNotebooks will get all the user notebooks and store them in the struct
func (u *User) LoadNotebooks() {
	defer u.SetViewState(StateViewNotebooks)
	r, err := u.Get(URLGETNotebooks)
	if err != nil {
		log.Println("Getting notebooks failed")
	}

	json.Unmarshal(processResponse(r), &u.Notebooks)
	// log.Println(u.Notebooks)
}

// LoadSections gets data to populate the section view
func (u *User) LoadSections(n Notebook) {
	defer u.SetViewState(StateViewSections)

	r, err := u.Get(URLGETSections, n.ID)
	if err != nil {
		log.Println(err)
	}

	json.Unmarshal(processResponse(r), &u.Sections)
	// log.Println(u.Sections)
}

// LoadPages gets data to populate page list
func (u *User) LoadPages(s Section) {
	defer u.SetViewState(StateViewPages)

	r, err := u.Get(URLGETPages, s.ID)
	if err != nil {
		log.Println(err)
	}

	json.Unmarshal(processResponse(r), &u.Pages)
}

// LoadPage calls the api for the page data
func (u *User) LoadPage(p Page) {
	defer u.SetViewState(StateViewPage)

	r, err := u.Get(p.ContentURL)
	if err != nil {
		log.Println(err)
	}
	defer r.Body.Close()

	responseData, err := ioutil.ReadAll(r.Body)
	mv, err := mxj.NewMapXml(responseData)
	if err != nil {
		log.Println(err)
	}
	// log.Println(mv)
	// log.Println(mv.LeafPaths())
	// log.Println(mv.LeafValues())
	// log.Println(mv.Elements("html.body"))
	// u.CurrentPage.Content = "" //string(responseData)[1:100] //TODO: strip whitespace, parse
	u.CurrentPage.Content = mv
}

func pruneXML(m mxj.Map) {
	// htmlAllowed := []string{"html", "body", "div", "span", "p", "br"}
}

// processResponse grabs the API data and returns the byte steram to be
// Unmarshaled into the relevant structure
func processResponse(r *http.Response) []byte {
	defer r.Body.Close()

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(err)
	}

	var resp map[string]*json.RawMessage
	json.Unmarshal(data, &resp)
	return *resp["value"]
}

// LoggedIn checks to see if the user api token is working
func (u *User) LoggedIn() bool {
	if !u.Token.Valid() {
		log.Println("Token detected as invalid.")

		r, _ := http.Get("https://www.onenote.com/api/v1.0/me/notes/pages")
		if r.StatusCode != 200 {
			return false
		}
		return true
	}
	return true
}

// StartAuth is the start of the auth procedure
func (u *User) StartAuth() {
	if !u.LoggedIn() {
		log.Println("Need to Auth")
		state := randToken()

		l, err := net.Listen("tcp", ":12345")
		http.HandleFunc("/auth", makeHandlerFunc(handler, l))
		if err != nil {
			log.Println(err)
		}
		go http.Serve(l, nil)
		log.Println("Now awaiting authentication redirect...")
		// message = u.Config.AuthCodeURL(state)
		user.StateData = u.Config.AuthCodeURL(state)
		u.SetViewState(StateFinishAuthenticate)
	} else {
		log.Println("Already logged in")
		u.SetViewState(StateLoadNotebooks)
	}
}

// LogOut requests a logout via the api
func (u *User) LogOut() {
	// TODO: set RedirectURL to urn:ietf:wg:oauth:2.0:oob
	_, err := u.Get("https://login.live.com/oauth20_logout.srf?client_id=" + u.Config.ClientID)
	if err != nil {
		log.Println("Error!")
		return
	}
	log.Println("Signed out successfully...")
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

	// user.Window.Highlight = true
	user.Window.Cursor = true
	user.Window.BgColor = gocui.ColorBlack
	user.Window.SetManagerFunc(layout)

	if err := user.Window.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		log.Fatalln(err)
	}

	user.Window.SetKeybinding("", 'j', gocui.ModNone, cursorDownHandler)
	user.Window.SetKeybinding("", gocui.KeyArrowDown, gocui.ModNone, cursorDownHandler)

	user.Window.SetKeybinding("", 'k', gocui.ModNone, cursorUpHandler)
	user.Window.SetKeybinding("", gocui.KeyArrowUp, gocui.ModNone, cursorUpHandler)

	user.Window.SetKeybinding("", 'b', gocui.ModNone, backHandler)

	user.Window.SetKeybinding("", 'q', gocui.ModNone, quit)

	user.Window.SetKeybinding("", gocui.KeyEnter, gocui.ModNone, selectHandler)

	if err := user.Window.MainLoop(); err != nil && err != gocui.ErrQuit {
		log.Println(err)
	}

	log.Println("===========================================")
}

func backHandler(g *gocui.Gui, v *gocui.View) error {
	switch user.CurrentViewState {
	case StateViewNotebooks:
		// Nowhere to go
		break
	case StateViewSections:
		// Go to notebooks
		// user.SetViewState(StateLoadNotebooks)
		user.CurrentViewState = StateLoadNotebooks
		break
	case StateViewPages:
		// Go to sections
		// user.SetViewState(StateLoadSections)
		user.CurrentViewState = StateLoadSections
		break
	case StateViewPage:
		// Go to pages
		user.CurrentViewState = StateLoadPages
		break
	}
	return nil
}

func selectHandler(g *gocui.Gui, v *gocui.View) error {
	states := []ViewState{StateViewNotebooks, StateViewSections, StateViewPages}

	if user.inState(states) && g.CurrentView() != nil {
		vw := g.CurrentView()
		switch user.CurrentViewState {
		case StateViewNotebooks:
			_, ind := vw.Cursor()
			nb := user.Notebooks[ind]
			user.StateData = nb.Name
			log.Printf("Selected notebook: %s\n", nb.Name)
			user.CurrentNotebook = nb
			user.CurrentViewState = StateLoadSections
			// user.SetViewState(StateLoadSections)
			break
		case StateViewSections:
			_, ind := vw.Cursor()
			sec := user.Sections[ind]
			user.StateData = sec.Name
			user.CurrentSection = sec
			user.CurrentViewState = StateLoadPages
			break
		case StateViewPages:
			_, ind := vw.Cursor()
			p := user.Pages[ind]
			user.StateData = p.Title
			user.CurrentPage = p
			user.CurrentViewState = StateLoadPage
			break
		}
	}
	return nil
}

func (u *User) inState(arr []ViewState) bool {
	for _, v := range arr {
		if u.CurrentViewState == v {
			return true
		}
	}
	return false
}

func cursorDownHandler(g *gocui.Gui, v *gocui.View) error {
	states := []ViewState{StateViewNotebooks, StateViewSections, StateViewPages}
	if user.inState(states) && g.CurrentView() != nil {
		_, y := g.CurrentView().Cursor()
		var max int
		switch user.CurrentViewState {
		case StateViewNotebooks:
			max = len(user.Notebooks) - 1
			break
		case StateViewSections:
			max = len(user.Sections) - 1
			break
		case StateViewPages:
			max = len(user.Pages) - 1
			break
		}
		if y >= max {
			return nil
		}
		g.CurrentView().MoveCursor(0, 1, false)
	}
	return nil
}

func cursorUpHandler(g *gocui.Gui, v *gocui.View) error {
	states := []ViewState{StateViewNotebooks, StateViewSections, StateViewPages}

	if user.inState(states) && g.CurrentView() != nil {
		g.CurrentView().MoveCursor(0, -1, false)
	}
	return nil
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
	user.SetViewState(StateLoadNotebooks)
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
	log.Println("Calling layout function...")
	log.Printf("Current user state: %s\n", viewStateName[user.CurrentViewState])
	// log.Println("Current user state data:")
	// log.Println(user.StateData)
	maxX, maxY := g.Size()
	var v *gocui.View
	var err error
	switch user.CurrentViewState {
	case StateStartAuthenticate:
		v, err = g.SetView("signin_link", 0, maxY/2-10, maxX-1, maxY/2+10)
		if err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.Title = "Sign In"
			v.Wrap = true
			fmt.Fprintln(v, "Getting authentication link...")
			go user.StartAuth()
		}
		break
	case StateFinishAuthenticate:
		v, err = g.View("signin_link")
		if err != nil {
			log.Println(err)
		}
		v.Title = "Sign In"
		v.Wrap = true
		v.Clear()
		fmt.Fprintln(v, user.StateData)
		log.Println("Authentication link:\n" + user.StateData)
		break
	case StateLoadNotebooks:
		v, err = g.SetView("notebooks", 0, 0, maxX-1, maxY-1)
		if err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.Title = "Notebooks"
			fmt.Fprintln(v, "Loading notebooks...")
		}
		go user.LoadNotebooks()
		break
	case StateViewNotebooks:
		v, err = g.View("notebooks")
		if err != nil {
			log.Println(err)
		}
		v.Title = "Notebooks"
		v.Clear()
		v.Highlight = true
		v.SelBgColor = gocui.Attribute(termbox.ColorWhite)
		v.SelFgColor = gocui.Attribute(termbox.ColorBlack)
		for _, n := range user.Notebooks {
			fmt.Fprintln(v, n.Name)
		}
		break
	case StateLoadSections:
		v, err = g.SetView("sections", 0, 0, maxX-1, maxY-1)
		if err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.Title = "Sections"
			fmt.Fprintln(v, "Loading sections...")
		}
		go user.LoadSections(user.CurrentNotebook)
		break
	case StateViewSections:
		v, err = g.View("sections")
		if err != nil {
			log.Println(err)
		}
		v.Title = user.CurrentNotebook.Name + " - Sections"
		v.Clear()
		v.Highlight = true
		v.SelBgColor = gocui.Attribute(termbox.ColorWhite)
		v.SelFgColor = gocui.Attribute(termbox.ColorBlack)
		for _, s := range user.Sections {
			fmt.Fprintln(v, s.Name)
		}
		break
	case StateLoadPages:
		v, err = g.SetView("pages", 0, 0, maxX-1, maxY-1)
		if err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.Title = "Pages"
			fmt.Fprintln(v, "Loading pages...")
		}
		go user.LoadPages(user.CurrentSection)
		break
	case StateViewPages:
		v, err = g.View("pages")
		if err != nil {
			log.Println(err)
		}
		v.Title = user.CurrentNotebook.Name + " - " + user.CurrentSection.Name + " - Pages"
		v.Clear()
		v.Highlight = true
		v.SelBgColor = gocui.Attribute(termbox.ColorWhite)
		v.SelFgColor = gocui.Attribute(termbox.ColorBlack)
		for _, p := range user.Pages {
			fmt.Fprintln(v, p.Title)
		}
		break
	case StateLoadPage:
		v, err = g.SetView("page", 0, 0, maxX-1, maxY-1)
		if err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.Title = user.CurrentPage.Title
			fmt.Fprintln(v, "Loading page...")
		}
		v.Clear()
		go user.LoadPage(user.CurrentPage)
		break
	case StateViewPage:
		v, err = g.View("page")
		if err != nil {
			log.Println(err)
		}
		v.Title = user.CurrentPage.Title
		v.Clear()
		v.Highlight = true
		v.Wrap = true
		v.SelBgColor = gocui.Attribute(termbox.ColorWhite)
		v.SelFgColor = gocui.Attribute(termbox.ColorBlack)
		// fmt.Fprintln(v, user.CurrentPage.Content)
		user.RenderCurrentPage(v)
		break
	}

	focus(g, v)

	return nil
}

// RenderCurrentPage prints the XML
func (u *User) RenderCurrentPage(v *gocui.View) {
	vals, err := u.CurrentPage.Content.ValuesForKey("#text")
	if err != nil {
		log.Println(err)
	}
	for _, val := range vals {
		log.Println(val)
		fmt.Fprintln(v, val)
	}
}

func focus(g *gocui.Gui, v *gocui.View) {
	vX, vY := v.Size()

	g.SetCurrentView(v.Name())
	g.SetViewOnTop(v.Name())
	v.SetCursor(vX, vY)
}

// GetPages set up the view of pages
func (u *User) GetPages(g *gocui.Gui) {
	pages, err := u.Get("https://www.onenote.com/api/v1.0/me/notes/pages")
	if err != nil {
		log.Fatal(err)
	}
	log.Println(pages)
}

func quit(g *gocui.Gui, v *gocui.View) error {
	log.Println("Quitting")
	return gocui.ErrQuit
}
