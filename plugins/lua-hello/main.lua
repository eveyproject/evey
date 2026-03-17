local plugin_id = os.getenv("EVEY_PLUGIN_ID") or "lua-hello"
local sock = os.getenv("EVEY_PLUGIN_SOCK") or "/tmp/evey/plugins/lua-hello.sock"

io.stderr:write("[" .. plugin_id .. "] listening on " .. sock .. "\n")

-- Lua has no built-in signal handling; rely on the host to kill the process
while true do
    os.execute("sleep 5")
end
