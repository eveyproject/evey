package demo

// MediaFile is one freely licensed file the demo bootstrap downloads into the
// demo media directory. Path is relative to that directory; its first segment
// is the library folder. ZipMember names the file to extract when URL is a
// zip archive.
type MediaFile struct {
	URL       string
	Path      string
	ZipMember string
	// Build, instead of URL, assembles Path from several official downloads:
	// the files nobody publishes ready-made (a Matroska with several audio
	// tracks, embedded subtitles and chapters) are made by the demo itself.
	Build *Build
}

// Build is one file muxed by ffmpeg from Inputs, in order. Args go between
// the inputs and the output (maps, per-track codecs and metadata); the video
// is always copied, so the only work is the odd audio track encoded once.
// Chapters, when set, become the file's chapter marks; the last one ends at
// DurationSec (ffmpeg does not clamp a later end in Matroska).
type Build struct {
	Inputs      []BuildInput
	Args        []string
	Chapters    []Chapter
	DurationSec int
}

// BuildInput is one download a Build reads. Ext keeps the extension ffmpeg
// probes the format from.
type BuildInput struct {
	URL       string
	ZipMember string
	Ext       string
}

// Chapter is a titled mark, StartSec seconds into the file.
type Chapter struct {
	StartSec int
	Title    string
}

// RetiredMedia were downloaded by an earlier demo and are deleted at boot,
// so a title replaced by a richer file does not show twice.
var RetiredMedia = []string{
	"movies/Tears of Steel (2012).webm",
	"movies/Matroska Test 4 - Live Stream (2008).mkv",
}

// Library is one of the four demo libraries: its display name, the library
// type (the domain's LibraryType string) and the folder under the media dir.
type Library struct {
	Name   string
	Type   string
	Folder string
	// FormerNames are names a running demo may still carry: the bootstrap
	// renames such a library (when it points at Folder) instead of creating a
	// second one, and the teardown still recognises it.
	FormerNames []string
}

// Libraries are created at boot when missing, by name.
var Libraries = []Library{
	{Name: "Movies", Type: "movie", Folder: "movies", FormerNames: []string{"Demo - Films"}},
	{Name: "Shows", Type: "tvshow", Folder: "series", FormerNames: []string{"Demo - Séries"}},
	{Name: "Music", Type: "music", Folder: "music", FormerNames: []string{"Demo - Musique"}},
	{Name: "Photos", Type: "photo", Folder: "photos", FormerNames: []string{"Demo - Photos"}},
}

// Manifest is the demo catalogue, every entry redistributable: the Blender
// open movies (CC-BY), public-domain classics and television series from
// archive.org, Kevin MacLeod's tracks (CC-BY) and Unsplash-licensed photos. The
// quick libraries come first so the demo has something to show while the
// large films are still arriving. Names carry the year so the scanner matches
// the right title; the two Sintel entries put a real second audio language
// (the 1080p master carries 5.1 English and ten embedded subtitles, the sidecar
// adds the Japanese one) and Big Buck Bunny is the 4K 60 fps master with a
// stereo and a 5.1 track.
var Manifest = []MediaFile{
	// Photos, in three albums: the home page and the photo library show
	// albums, a flat folder shows nothing there.
	{URL: "https://picsum.photos/seed/evey1/1600/1067.jpg", Path: "photos/Voyage/Demo 01.jpg"},
	{URL: "https://picsum.photos/seed/evey2/1600/1067.jpg", Path: "photos/Voyage/Demo 02.jpg"},
	{URL: "https://picsum.photos/seed/evey3/1600/1067.jpg", Path: "photos/Voyage/Demo 03.jpg"},
	{URL: "https://picsum.photos/seed/evey4/1600/1067.jpg", Path: "photos/Voyage/Demo 04.jpg"},
	{URL: "https://picsum.photos/seed/evey5/1600/1067.jpg", Path: "photos/Voyage/Demo 05.jpg"},
	{URL: "https://picsum.photos/seed/evey6/1600/1067.jpg", Path: "photos/Voyage/Demo 06.jpg"},
	{URL: "https://picsum.photos/seed/evey7/1600/1067.jpg", Path: "photos/Voyage/Demo 07.jpg"},
	{URL: "https://picsum.photos/seed/evey8/1600/1067.jpg", Path: "photos/Voyage/Demo 08.jpg"},
	{URL: "https://picsum.photos/seed/evey9/1600/1067.jpg", Path: "photos/Nature/Demo 09.jpg"},
	{URL: "https://picsum.photos/seed/evey10/1600/1067.jpg", Path: "photos/Nature/Demo 10.jpg"},
	{URL: "https://picsum.photos/seed/evey11/1600/1067.jpg", Path: "photos/Nature/Demo 11.jpg"},
	{URL: "https://picsum.photos/seed/evey12/1600/1067.jpg", Path: "photos/Nature/Demo 12.jpg"},
	{URL: "https://picsum.photos/seed/evey13/1600/1067.jpg", Path: "photos/Nature/Demo 13.jpg"},
	{URL: "https://picsum.photos/seed/evey14/1600/1067.jpg", Path: "photos/Nature/Demo 14.jpg"},
	{URL: "https://picsum.photos/seed/evey15/1600/1067.jpg", Path: "photos/Nature/Demo 15.jpg"},
	{URL: "https://picsum.photos/seed/evey16/1600/1067.jpg", Path: "photos/Nature/Demo 16.jpg"},
	{URL: "https://picsum.photos/seed/evey17/1600/1067.jpg", Path: "photos/Ville/Demo 17.jpg"},
	{URL: "https://picsum.photos/seed/evey18/1600/1067.jpg", Path: "photos/Ville/Demo 18.jpg"},
	{URL: "https://picsum.photos/seed/evey19/1600/1067.jpg", Path: "photos/Ville/Demo 19.jpg"},
	{URL: "https://picsum.photos/seed/evey20/1600/1067.jpg", Path: "photos/Ville/Demo 20.jpg"},
	{URL: "https://picsum.photos/seed/evey21/1600/1067.jpg", Path: "photos/Ville/Demo 21.jpg"},
	{URL: "https://picsum.photos/seed/evey22/1600/1067.jpg", Path: "photos/Ville/Demo 22.jpg"},
	{URL: "https://picsum.photos/seed/evey23/1600/1067.jpg", Path: "photos/Ville/Demo 23.jpg"},
	{URL: "https://picsum.photos/seed/evey24/1600/1067.jpg", Path: "photos/Ville/Demo 24.jpg"},

	// Music - Kevin MacLeod, CC-BY 4.0
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Monkeys%20Spinning%20Monkeys.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 1/01 - Monkeys Spinning Monkeys.mp3"},
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Sneaky%20Snitch.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 1/02 - Sneaky Snitch.mp3"},
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Carefree.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 1/03 - Carefree.mp3"},
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Fluffing%20a%20Duck.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 1/04 - Fluffing a Duck.mp3"},
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Wallpaper.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 1/05 - Wallpaper.mp3"},
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Pixel%20Peeker%20Polka%20-%20faster.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 1/06 - Pixel Peeker Polka - faster.mp3"},
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Scheming%20Weasel%20faster.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 1/07 - Scheming Weasel faster.mp3"},
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Investigations.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 1/08 - Investigations.mp3"},
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Local%20Forecast%20-%20Elevator.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 2/01 - Local Forecast - Elevator.mp3"},
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Danse%20Macabre.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 2/02 - Danse Macabre.mp3"},
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Arcadia.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 2/03 - Arcadia.mp3"},
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Amazing%20Plan.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 2/04 - Amazing Plan.mp3"},
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Kick%20Shock.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 2/05 - Kick Shock.mp3"},
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Hyperfun.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 2/06 - Hyperfun.mp3"},
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Meatball%20Parade.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 2/07 - Meatball Parade.mp3"},
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Sock%20Hop.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 2/08 - Sock Hop.mp3"},
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Life%20of%20Riley.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 2/09 - Life of Riley.mp3"},
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Deliberate%20Thought.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 2/10 - Deliberate Thought.mp3"},
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Thinking%20Music.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 2/11 - Thinking Music.mp3"},
	{URL: "https://incompetech.com/music/royalty-free/mp3-royaltyfree/Bossa%20Antigua.mp3", Path: "music/Kevin MacLeod/Royalty Free Vol. 2/12 - Bossa Antigua.mp3"},

	// Series - Sherlock Holmes (1954), public domain
	// Colonel Bleep (1957), the first colour TV animated series, in
	// the public domain (Public Domain Mark on archive.org; 17 surviving
	// five-minute episodes).
	{URL: "https://archive.org/download/colonel-bleep-vol2-s01e01-17/Colonel%20Bleep%20Volume%202/S01E01%20The%20Firebomb.mp4", Path: "series/Colonel Bleep (1957)/Season 01/Colonel Bleep (1957) - S01E01 - The Firebomb.mp4"},
	{URL: "https://archive.org/download/colonel-bleep-vol2-s01e01-17/Colonel%20Bleep%20Volume%202/S01E02%20The%20Ghastly%20Ghost.mp4", Path: "series/Colonel Bleep (1957)/Season 01/Colonel Bleep (1957) - S01E02 - The Ghastly Ghost.mp4"},
	{URL: "https://archive.org/download/colonel-bleep-vol2-s01e01-17/Colonel%20Bleep%20Volume%202/S01E03%20The%20Greedy%20Gorilla.mp4", Path: "series/Colonel Bleep (1957)/Season 01/Colonel Bleep (1957) - S01E03 - The Greedy Gorilla.mp4"},
	{URL: "https://archive.org/download/colonel-bleep-vol2-s01e01-17/Colonel%20Bleep%20Volume%202/S01E04%20The%20Hypnotic%20Helmets.mp4", Path: "series/Colonel Bleep (1957)/Season 01/Colonel Bleep (1957) - S01E04 - The Hypnotic Helmets.mp4"},
	{URL: "https://archive.org/download/colonel-bleep-vol2-s01e01-17/Colonel%20Bleep%20Volume%202/S01E05%20The%20Ingenious%20Invention.mp4", Path: "series/Colonel Bleep (1957)/Season 01/Colonel Bleep (1957) - S01E05 - The Ingenious Invention.mp4"},
	{URL: "https://archive.org/download/colonel-bleep-vol2-s01e01-17/Colonel%20Bleep%20Volume%202/S01E06%20The%20Invisible%20Gorilla.mp4", Path: "series/Colonel Bleep (1957)/Season 01/Colonel Bleep (1957) - S01E06 - The Invisible Gorilla.mp4"},
	{URL: "https://archive.org/download/colonel-bleep-vol2-s01e01-17/Colonel%20Bleep%20Volume%202/S01E07%20The%20Killer%20Whale.mp4", Path: "series/Colonel Bleep (1957)/Season 01/Colonel Bleep (1957) - S01E07 - The Killer Whale.mp4"},
	{URL: "https://archive.org/download/colonel-bleep-vol2-s01e01-17/Colonel%20Bleep%20Volume%202/S01E08%20The%20Lunar%20Luger.mp4", Path: "series/Colonel Bleep (1957)/Season 01/Colonel Bleep (1957) - S01E08 - The Lunar Luger.mp4"},
	{URL: "https://archive.org/download/colonel-bleep-vol2-s01e01-17/Colonel%20Bleep%20Volume%202/S01E09%20The%20Pirate%20Plot.mp4", Path: "series/Colonel Bleep (1957)/Season 01/Colonel Bleep (1957) - S01E09 - The Pirate Plot.mp4"},
	{URL: "https://archive.org/download/colonel-bleep-vol2-s01e01-17/Colonel%20Bleep%20Volume%202/S01E10%20The%20Prehistoric%20Present.mp4", Path: "series/Colonel Bleep (1957)/Season 01/Colonel Bleep (1957) - S01E10 - The Prehistoric Present.mp4"},
	{URL: "https://archive.org/download/colonel-bleep-vol2-s01e01-17/Colonel%20Bleep%20Volume%202/S01E11%20The%20Runaway%20Rocket.mp4", Path: "series/Colonel Bleep (1957)/Season 01/Colonel Bleep (1957) - S01E11 - The Runaway Rocket.mp4"},
	{URL: "https://archive.org/download/colonel-bleep-vol2-s01e01-17/Colonel%20Bleep%20Volume%202/S01E12%20The%20Treacherous%20Pirate.mp4", Path: "series/Colonel Bleep (1957)/Season 01/Colonel Bleep (1957) - S01E12 - The Treacherous Pirate.mp4"},
	{URL: "https://archive.org/download/colonel-bleep-vol2-s01e01-17/Colonel%20Bleep%20Volume%202/S01E13%20The%20Treacherous%20Trio.mp4", Path: "series/Colonel Bleep (1957)/Season 01/Colonel Bleep (1957) - S01E13 - The Treacherous Trio.mp4"},
	{URL: "https://archive.org/download/colonel-bleep-vol2-s01e01-17/Colonel%20Bleep%20Volume%202/S01E14%20Uncharted%20Island.mp4", Path: "series/Colonel Bleep (1957)/Season 01/Colonel Bleep (1957) - S01E14 - Uncharted Island.mp4"},
	{URL: "https://archive.org/download/colonel-bleep-vol2-s01e01-17/Colonel%20Bleep%20Volume%202/S01E15%20The%20Wicked%20Web.mp4", Path: "series/Colonel Bleep (1957)/Season 01/Colonel Bleep (1957) - S01E15 - The Wicked Web.mp4"},
	{URL: "https://archive.org/download/colonel-bleep-vol2-s01e01-17/Colonel%20Bleep%20Volume%202/S01E16%20Tunnel%20in%20Space.mp4", Path: "series/Colonel Bleep (1957)/Season 01/Colonel Bleep (1957) - S01E16 - Tunnel in Space.mp4"},
	{URL: "https://archive.org/download/colonel-bleep-vol2-s01e01-17/Colonel%20Bleep%20Volume%202/S01E17%20War%20in%20Robot%20Land.mp4", Path: "series/Colonel Bleep (1957)/Season 01/Colonel Bleep (1957) - S01E17 - War in Robot Land.mp4"},
	{URL: "https://archive.org/download/SherlockHolmes1954/Sherlock%20Holmes%2001%20The%20Case%20of%20the%20Cunningham%20Heritage.mp4", Path: "series/Sherlock Holmes (1954)/Season 01/Sherlock Holmes (1954) - S01E01 - The Case of the Cunningham Heritage.mp4"},
	{URL: "https://archive.org/download/SherlockHolmes1954/Sherlock%20Holmes%2002%20The%20Case%20Of%20Lady%20Beryl.mp4", Path: "series/Sherlock Holmes (1954)/Season 01/Sherlock Holmes (1954) - S01E02 - The Case of Lady Beryl.mp4"},
	{URL: "https://archive.org/download/SherlockHolmes1954/Sherlock%20Holmes%2003%20The%20Case%20of%20the%20Pennsylvania%20Gun.mp4", Path: "series/Sherlock Holmes (1954)/Season 01/Sherlock Holmes (1954) - S01E03 - The Case of the Pennsylvania Gun.mp4"},

	// Series - The Beverly Hillbillies (1962) and Bonanza (1959), public-domain marked episodes
	{URL: "https://archive.org/download/bevhill-s01e01-36/s01e01-The-Clampetts-Strike-Oil.mp4", Path: "series/The Beverly Hillbillies (1962)/Season 01/The Beverly Hillbillies (1962) - S01E01 - The Clampetts Strike Oil.mp4"},
	{URL: "https://archive.org/download/bevhill-s01e01-36/s01e02-Getting-Settled.mp4", Path: "series/The Beverly Hillbillies (1962)/Season 01/The Beverly Hillbillies (1962) - S01E02 - Getting Settled.mp4"},
	{URL: "https://archive.org/download/bevhill-s01e01-36/s01e03_Meanwhile_Back_At_The_Cabin.mp4", Path: "series/The Beverly Hillbillies (1962)/Season 01/The Beverly Hillbillies (1962) - S01E03 - Meanwhile Back at the Cabin.mp4"},
	{URL: "https://archive.org/download/bevhill-s01e01-36/s01e04-The-Clampetts-Meet-Mrs-Drysdale.mp4", Path: "series/The Beverly Hillbillies (1962)/Season 01/The Beverly Hillbillies (1962) - S01E04 - The Clampetts Meet Mrs. Drysdale.mp4"},
	{URL: "https://archive.org/download/Bonanza_pd/Bonanza%20s01e19.mp4", Path: "series/Bonanza (1959)/Season 01/Bonanza (1959) - S01E19.mp4"},
	{URL: "https://archive.org/download/Bonanza_pd/Bonanza%20s01e20.mp4", Path: "series/Bonanza (1959)/Season 01/Bonanza (1959) - S01E20.mp4"},
	{URL: "https://archive.org/download/Bonanza_pd/Bonanza%20s01e21.mp4", Path: "series/Bonanza (1959)/Season 01/Bonanza (1959) - S01E21.mp4"},

	// Films - Blender open movies, CC-BY
	{URL: "https://archive.org/download/caminandes-1-llama-drama/1_Caminandes1_Llama_Drama_720p_vp8.ogv", Path: "movies/Caminandes - Llama Drama (2013).ogv"},
	{URL: "https://archive.org/download/Caminandes2GranDillama/02_gran_dillama_1080p.mp4", Path: "movies/Caminandes 2 - Gran Dillama (2013).mp4"},
	{URL: "https://archive.org/download/CaminandesLlamigos/Caminandes_%20Llamigos-1080p.mp4", Path: "movies/Caminandes 3 - Llamigos (2016).mp4"},
	{URL: "https://upload.wikimedia.org/wikipedia/commons/a/a5/Spring_-_Blender_Open_Movie.webm", Path: "movies/Spring (2019).webm"},
	{URL: "https://archive.org/download/sprite-fright-2021/Sprite%20Fright%20%282021%29.mkv", Path: "movies/Sprite Fright (2021).mkv"},
	{URL: "https://archive.org/download/CosmosLaundromatFirstCycle/Cosmos%20Laundromat%20-%20First%20Cycle%20%281080p%29.mp4", Path: "movies/Cosmos Laundromat (2015).mp4"},
	// Tears of Steel as a Matroska made at boot from the official files:
	// H264 copied, the stereo MP3, the official Dolby Digital 5.1 mix, the
	// same mix as Dolby Digital Plus (the one track encoded, once), twelve
	// official subtitle languages and evenly spaced chapter marks - several audio tracks,
	// embedded subtitles and AC-3/E-AC-3 in one container.
	{Path: "movies/Tears of Steel (2012).mkv", Build: &Build{
		// Twelve of the official subtitle files: the Portuguese ones are left
		// out, both write their timings "->" (not SRT) and ffmpeg refuses them.
		Inputs: []BuildInput{
			{URL: "https://download.blender.org/demo/movies/ToS/tears_of_steel_720p.mov", Ext: ".mov"},
			{URL: "https://download.blender.org/demo/movies/ToS/Surround-TOS_DVDSURROUND-Dolby%205.1.ac3", Ext: ".ac3"},
			{URL: "https://download.blender.org/demo/movies/ToS/subtitles/TOS-en.srt", Ext: ".srt"},
			{URL: "https://download.blender.org/demo/movies/ToS/subtitles/TOS-fr-orig.srt", Ext: ".srt"},
			{URL: "https://download.blender.org/demo/movies/ToS/subtitles/TOS-de.srt", Ext: ".srt"},
			{URL: "https://download.blender.org/demo/movies/ToS/subtitles/TOS-es.srt", Ext: ".srt"},
			{URL: "https://download.blender.org/demo/movies/ToS/subtitles/TOS-it.srt", Ext: ".srt"},
			{URL: "https://download.blender.org/demo/movies/ToS/subtitles/TOS-nl.srt", Ext: ".srt"},
			{URL: "https://download.blender.org/demo/movies/ToS/subtitles/TOS-GK.srt", Ext: ".srt"},
			{URL: "https://download.blender.org/demo/movies/ToS/subtitles/TOS-ru.srt", Ext: ".srt"},
			{URL: "https://download.blender.org/demo/movies/ToS/subtitles/TOS-JP.srt", Ext: ".srt"},
			{URL: "https://download.blender.org/demo/movies/ToS/subtitles/TOS-CZ.srt", Ext: ".srt"},
			{URL: "https://download.blender.org/demo/movies/ToS/subtitles/TOS-HU.srt", Ext: ".srt"},
			{URL: "https://download.blender.org/demo/movies/ToS/subtitles/TOS-HE.srt", Ext: ".srt"},
		},
		Args: []string{
			"-map", "0:v:0", "-map", "0:a:0", "-map", "1:a:0", "-map", "1:a:0",
			"-map", "2:s:0",
			"-map", "3:s:0",
			"-map", "4:s:0",
			"-map", "5:s:0",
			"-map", "6:s:0",
			"-map", "7:s:0",
			"-map", "8:s:0",
			"-map", "9:s:0",
			"-map", "10:s:0",
			"-map", "11:s:0",
			"-map", "12:s:0",
			"-map", "13:s:0",
			"-c", "copy", "-c:a:2", "eac3", "-b:a:2", "384k",
			"-metadata", "title=Tears of Steel",
			"-metadata:s:v:0", "language=eng",
			"-metadata:s:a:0", "language=eng", "-metadata:s:a:0", "title=Stereo (MP3)", "-disposition:a:0", "default",
			"-metadata:s:a:1", "language=eng", "-metadata:s:a:1", "title=Dolby Digital 5.1", "-disposition:a:1", "0",
			"-metadata:s:a:2", "language=eng", "-metadata:s:a:2", "title=Dolby Digital Plus 5.1", "-disposition:a:2", "0",
			"-metadata:s:s:0", "language=eng", "-metadata:s:s:0", "title=English", "-disposition:s:0", "0",
			"-metadata:s:s:1", "language=fre", "-metadata:s:s:1", "title=Français", "-disposition:s:1", "0",
			"-metadata:s:s:2", "language=ger", "-metadata:s:s:2", "title=Deutsch", "-disposition:s:2", "0",
			"-metadata:s:s:3", "language=spa", "-metadata:s:s:3", "title=Español", "-disposition:s:3", "0",
			"-metadata:s:s:4", "language=ita", "-metadata:s:s:4", "title=Italiano", "-disposition:s:4", "0",
			"-metadata:s:s:5", "language=dut", "-metadata:s:s:5", "title=Nederlands", "-disposition:s:5", "0",
			"-metadata:s:s:6", "language=gre", "-metadata:s:s:6", "title=Ελληνικά", "-disposition:s:6", "0",
			"-metadata:s:s:7", "language=rus", "-metadata:s:s:7", "title=Русский", "-disposition:s:7", "0",
			"-metadata:s:s:8", "language=jpn", "-metadata:s:s:8", "title=日本語", "-disposition:s:8", "0",
			"-metadata:s:s:9", "language=cze", "-metadata:s:s:9", "title=Čeština", "-disposition:s:9", "0",
			"-metadata:s:s:10", "language=hun", "-metadata:s:s:10", "title=Magyar", "-disposition:s:10", "0",
			"-metadata:s:s:11", "language=heb", "-metadata:s:s:11", "title=עברית", "-disposition:s:11", "0",
		},
		Chapters: []Chapter{
			{StartSec: 0, Title: "Chapter 01"},
			{StartSec: 120, Title: "Chapter 02"},
			{StartSec: 240, Title: "Chapter 03"},
			{StartSec: 360, Title: "Chapter 04"},
			{StartSec: 480, Title: "Chapter 05"},
			{StartSec: 600, Title: "Chapter 06"},
		},
		DurationSec: 734,
	}},
	{URL: "https://commons.wikimedia.org/w/api.php?action=timedtext&title=File:Sintel_movie_4K.webm&lang=ja&trackformat=srt", Path: "movies/Sintel (2010).ja.srt"},
	{URL: "https://download.blender.org/demo/movies/Sintel.2010.1080p.mkv", Path: "movies/Sintel (2010).mkv"},
	{URL: "https://upload.wikimedia.org/wikipedia/commons/a/a2/Elephants_Dream_%282006%29.webm", Path: "movies/Elephants Dream (2006).webm"},
	{URL: "https://download.blender.org/demo/movies/BBB/bbb_sunflower_2160p_60fps_normal.mp4.zip", Path: "movies/Big Buck Bunny (2008).mp4", ZipMember: "bbb_sunflower_2160p_60fps_normal.mp4"},

	// Matroska's own test suite (IETF CELLAR, github.com/ietf-wg-cellar/
	// matroska-test-files): Elephants Dream, CC-BY, cut to exercise the parser.
	// test5 has two audio tracks (the commentary in AAC+ SBR) and subtitles in
	// seven languages. The damaged test7 and test8 stay in the integration
	// tests, never in the demo. test4 (a live recording: Clusters of unknown
	// size, no Duration) is held back: the packager cannot read it yet and the
	// server has no duration to build its playlist from, so it would show a
	// film that does not start.
	{URL: "https://raw.githubusercontent.com/ietf-wg-cellar/matroska-test-files/master/test_files/test5.mkv", Path: "movies/Matroska Test 5 - Multiple Audio and Subtitles (2006).mkv"},

	// Films - public domain classics (archive.org, public-domain marked items)
	{URL: "https://archive.org/download/his_girl_friday/his_girl_friday.mp4", Path: "movies/His Girl Friday (1940).mp4"},
	{URL: "https://archive.org/download/CarnivalOfSoulsVideoQualityUpgrade/CarnivalOfSoulsVideoUpgrade.mp4", Path: "movies/Carnival of Souls (1962).mp4"},
	{URL: "https://archive.org/download/HouseOnHauntedHillWidescreenVideoUpgrade/HouseOnHauntedHillWidescreenVideoUpgrade.mp4", Path: "movies/House on Haunted Hill (1959).mp4"},
	{URL: "https://archive.org/download/TheLittleShopOfHorrors1960_765/TheLittleShopOfHorrors1960.mp4", Path: "movies/The Little Shop of Horrors (1960).mp4"},
	{URL: "https://archive.org/download/The_General_Buster_Keaton/The_General.mp4", Path: "movies/The General (1926).mp4"},
	{URL: "https://archive.org/download/Metropolis1927EnglishVersion/Metropolis_1927_English_Version.mp4", Path: "movies/Metropolis (1927).mp4"},
}

// Plugin is one plugin the demo bootstrap installs so the plugin panel and
// the home slots have something to show. File is the bundle's name under the
// demo plugin directory (EVEY_DEMO_PLUGIN_DIR): the bundle is built from
// examples/plugins/<Name> and dropped there by the operator, the bootstrap
// installs, enables and configures it, and skips it when the file is absent.
// Config is applied at every boot, so a restore puts the plugin back the way
// the demo shows it.
type Plugin struct {
	Name   string
	File   string
	Config map[string]any
}

// Plugins are installed at boot when missing, by name. Only visual on the
// demo: every plugin write except reading the config is locked (policy.go).
// The bundle is built with tests/plugins/weather-v2/build-zip.sh.
var Plugins = []Plugin{
	{Name: "weather-v2", File: "weather-v2.zip", Config: map[string]any{"city": "Paris", "units": "celsius"}},
}

// RetiredPlugins were seeded by an earlier demo and are uninstalled at boot
// when found, so an upgraded demo does not show two weather widgets.
var RetiredPlugins = []string{"weather"}

// Home is the default home page of the demo, top to bottom: the weather
// widget, what the visitor was watching, the three media libraries by name,
// then the most popular titles. Every other section is disabled. The
// bootstrap writes it as the admin feed configuration at every boot; a
// visitor's own arrangement stays in their browser (the demo home layout).
var Home = []HomeSection{
	{Plugin: "weather-v2"},
	{Type: "resume_playback"},
	{Library: "Movies"},
	{Library: "Shows"},
	{Library: "Music"},
	{Type: "most_popular"},
}

// HiddenHomeTypes are the built-in sections the demo home does not show
// (the domain's other feed types); they stay in the admin panel, disabled.
var HiddenHomeTypes = []string{"continue_watching", "recently_added", "recommended", "up_next", "collections", "next_episode"}

// HomeSection names one home section by exactly one of: a feed type, a
// library display name (resolved to its id at boot) or a plugin name (same).
type HomeSection struct {
	Type    string
	Library string
	Plugin  string
}

// History is the watch history the bootstrap seeds on the demo profile, so
// the home's Resume, Next up and Recommended rows have something to show from
// the first boot and after every restore (the shared account's own viewing
// adds to it during the hour). Films are resumed at Percent of their length
// (Percent 0 = watched to the end); of a series, the first WatchedEpisodes of
// season 1 are watched, so Next up offers the following one, and with a
// Percent that following episode is resumed there too, so Resume mixes films
// and episodes the way a real account's does.
type HistoryEntry struct {
	Film            string
	Percent         int
	Series          string
	WatchedEpisodes int
}

var History = []HistoryEntry{
	{Film: "Big Buck Bunny"},
	{Film: "Elephants Dream"},
	{Film: "Caminandes: Llama Drama"},
	{Film: "Sintel", Percent: 40},
	{Film: "Spring", Percent: 60},
	{Film: "Tears of Steel", Percent: 25},
	{Film: "Cosmos Laundromat", Percent: 70},
	{Film: "Metropolis", Percent: 35},
	{Film: "His Girl Friday", Percent: 55},
	{Film: "The General", Percent: 15},
	{Series: "Colonel Bleep", WatchedEpisodes: 2, Percent: 50},
	{Series: "Sherlock Holmes", WatchedEpisodes: 1, Percent: 30},
	{Series: "Bonanza", Percent: 45},
	{Series: "The Beverly Hillbillies", WatchedEpisodes: 1},
}
