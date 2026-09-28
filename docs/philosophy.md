# Why Evey

You already own the films. You ripped the discs, kept the files, sorted the
folders. What is missing is the part that makes them pleasant to watch: a
poster wall, the episode you stopped at, the right audio track, the same
library on the TV, the phone and the laptop, and a way to share it with the
people you live with without handing it to a company.

Evey is that part. It runs on your machine - a NAS, a mini PC, an old laptop -
and serves your own files to your own screens. Five commitments shape every
decision in it, and when two of them conflict, this is the order in which they
win.

## 1. Safe to put on the internet

A media server is only useful if you can reach it from your sofa, from the
train and from your parents' house. So Evey is built to be reachable, not
hidden, and its defaults assume the network is hostile:

- remote access opens when you publish the server at a public address, and
  stays shut when you do not - you can always decide explicitly;
- when the server cannot check who you are (a database that does not answer,
  a parental setting it cannot read), it refuses rather than letting the
  request through;
- extensions only get the permissions their author declared, and the kernel
  confines them;
- the server never runs as the system administrator.

## 2. Installable in a few lines

One compose file, one container, no password to invent. The database and the
search engine live inside the container and generate their own secrets the
first time it starts. Everything you may want to change later - languages,
conversion limits, graphics card, schedules - is in the admin panel, not in a
configuration file you have to edit and restart.

## 3. Extensible in whatever language you write

Evey can be extended with plugins: metadata sources, home-page widgets,
themes. A plugin is a program the server starts and talks to over a local
connection, so it can be written in the language you already know -
ready-made kits exist for seven of them. Because extensions run in a sandbox
rather than being trusted, installing one written by somebody else is a
reasonable thing to do.

## 4. Fast, and honest about it

The quickest way to play a file is to send it as it is. Evey does that
whenever the device can read it, converts only the part the device cannot
handle (often just the sound), and decides once per playback instead of
second-guessing itself mid-film. The numbers behind those choices are measured
on real files and real machines, not assumed - an optimisation that looked
clever was kept only after it was proven to give the same answer as the tool it
replaced.

## 5. For many viewers at once, on modest hardware

The target is a household and its friends on a NAS, not a data centre. So
Evey measures what your machine can do the first time it starts, admits only
as many simultaneous conversions as it can carry, pauses its background work
while someone is watching, and sizes itself to the hardware it finds. A slow
machine plays fewer streams; it does not play all of them badly.

---

Two things Evey deliberately is not. It is not a catalogue of every media type
done adequately - playback is what it owns. And it is not a product that asks
you to trust it: what the server does, and what it still cannot prevent, is
written down in the [security model](SECURITY.md) rather than implied.

**Next:** [what you get](discover/features.md) ·
[the devices it plays on](discover/apps.md) ·
[install it](installation.md)
