<?php

$pluginId = getenv('EVEY_PLUGIN_ID') ?: 'php-hello';
$sock = getenv('EVEY_PLUGIN_SOCK') ?: '/tmp/evey/plugins/php-hello.sock';

pcntl_signal(SIGTERM, function () use ($pluginId, $sock) {
    fwrite(STDERR, "[$pluginId] SIGTERM received, shutting down\n");
    if (file_exists($sock)) {
        unlink($sock);
    }
    exit(0);
});
pcntl_signal(SIGINT, function () { posix_kill(posix_getpid(), SIGTERM); });

fwrite(STDERR, "[$pluginId] listening on $sock\n");

while (true) {
    sleep(5);
    pcntl_signal_dispatch();
}
