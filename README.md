# Duet – SSH Pair Programming

SSH-first pair programming tool built with Go + Charm (Bubbletea/Wish).

## Features

- Create/join pairing sessions via `ssh duet.jaypopat.me`
- Shared live terminal
- AI Agent and access to sandbox (cloudflare stack)
- Users can directly run commands on sandbox or let AI run commands through chat (some usecases include cloning a repo and operating file operations)

## How to access hosted version
Connect to the app using `ssh <username>@duet.jaypopat.me`
At the moment no security gates have been implemented so if you open a room, anyone can join!. This is the next problem to tackle. I made it this way to make it easy for development.

## How to run locally (DEV)
Run `make dev`

This will spin up the cloudflare worker locally (except the ai inference - happens on edge), and the go ssh server on port 2222.

Connect to this using the command `ssh <username>@localhost -p 2222`

---

Inspired by terminal.shop, I wanted to build SSH UIs. I’ve also been wanting to learn Go for a while, so I decided to build this project over a weekend to both learn Go and build something useful.