#!/usr/bin/env ruby

plugin_id = ENV.fetch('EVEY_PLUGIN_ID', 'ruby-hello')
sock = ENV.fetch('EVEY_PLUGIN_SOCK', '/tmp/evey/plugins/ruby-hello.sock')

Signal.trap('TERM') do
  $stderr.puts "[#{plugin_id}] SIGTERM received, shutting down"
  File.delete(sock) if File.exist?(sock)
  exit 0
end
Signal.trap('INT') { Process.kill('TERM', Process.pid) }

$stderr.puts "[#{plugin_id}] listening on #{sock}"

loop { sleep 5 }
