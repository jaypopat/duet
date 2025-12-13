# Testing Duet

## Quick Test

### Terminal 1 - Start Server
```bash
./bin/duet
# Server starts on :2222
```

### Terminal 2 - Host Creates Room
```bash
ssh localhost -p 2222
# Select "1" or press Enter to create room
# Note the Room ID (e.g., "a3f4b2c1")
# You'll get a shared bash shell
```

### Terminal 3 - Guest Joins Room
```bash
ssh localhost -p 2222
# Select "2" to join
# Enter the Room ID from Terminal 2
# Both terminals should now share the same shell!
```

## Testing with Neovim

In the **host terminal** (Terminal 2):
```bash
nvim test.txt
```

Both the host and guest should see the same Neovim session. Both can type and edit!

## Expected Behavior
- ✅ Both users see same terminal output
- ✅ Both users can type and send input
- ✅ Terminal resize works for host
- ✅ Vim/Neovim keybindings work
- ✅ Room closes when host disconnects

## Known Limitations (MVP)
- Only host window resize is synchronized
- No authentication (anyone can join with room ID)
- No chat/communication besides shared terminal
- Rooms are in-memory only (lost on restart)
