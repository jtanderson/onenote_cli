# OneNote CLI

This project is a terminal-based interface to the Microsoft
OneNote API. Why? It's fast, clean, and simple.

![screenshot](/img/6rr3WFy8VI.gif?raw=true "Screenshot")

## Status

Currently everything is read-only, but that aspect seems 
solidly usable. The sign-in process is slightly clunky in that
you either have to copy/paste the wrapped segments from the
display, or go to the logs where it is fully clickable if
you have a terminal that allows such things.

## TODO

- [ ] Basic navigation and reading of
    - [x] Notebooks
    - [x] Sections
    - [x] Pages
    - [ ] Section groups
- [ ] Allow creation and editing
  - [ ] Notebooks
  - [ ] Sections
  - [ ] Pages
  - [ ] Cache data for speed
- [ ] User preferences
  - [ ] Auth expiry
  - [ ] Cache frequency/laziness
- [ ] Rendering
  - [ ] Use `gocui.Execute` instead of two-stage data loading
  - [ ] Better text-selection
- [ ] Security
  - [ ] Propery handle app id and client secret (This is hindered by MS requiring use of url fragments and Golang actively prohibiting it)
