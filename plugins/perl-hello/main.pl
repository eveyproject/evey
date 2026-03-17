#!/usr/bin/env perl
use strict;
use warnings;

my $plugin_id = $ENV{EVEY_PLUGIN_ID} // 'perl-hello';
my $sock = $ENV{EVEY_PLUGIN_SOCK} // '/tmp/evey/plugins/perl-hello.sock';

$SIG{TERM} = sub {
    print STDERR "[$plugin_id] SIGTERM received, shutting down\n";
    unlink $sock if -e $sock;
    exit 0;
};
$SIG{INT} = $SIG{TERM};

print STDERR "[$plugin_id] listening on $sock\n";

while (1) {
    sleep 5;
}
