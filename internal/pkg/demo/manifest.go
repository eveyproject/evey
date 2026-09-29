package demo

// MediaFile is one freely licensed file the demo bootstrap downloads into the
// demo media directory. Path is relative to that directory; its first segment
// is the library folder. ZipMember names the file to extract when URL is a
// zip archive.
type MediaFile struct {
	URL       string
	Path      string
	ZipMember string
}

// Library is one of the four demo libraries: its display name, the library
// type (the domain's LibraryType string) and the folder under the media dir.
type Library struct {
	Name   string
	Type   string
	Folder string
}

// Libraries are created at boot when missing, by name.
var Libraries = []Library{
	{Name: "Demo - Films", Type: "movie", Folder: "movies"},
	{Name: "Demo - Séries", Type: "tvshow", Folder: "series"},
	{Name: "Demo - Musique", Type: "music", Folder: "music"},
	{Name: "Demo - Photos", Type: "photo", Folder: "photos"},
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
	{URL: "https://media.xiph.org/tearsofsteel/tears_of_steel_1080p.webm", Path: "movies/Tears of Steel (2012).webm"},
	{URL: "https://commons.wikimedia.org/w/api.php?action=timedtext&title=File:Sintel_movie_4K.webm&lang=ja&trackformat=srt", Path: "movies/Sintel (2010).ja.srt"},
	{URL: "https://download.blender.org/demo/movies/Sintel.2010.1080p.mkv", Path: "movies/Sintel (2010).mkv"},
	{URL: "https://upload.wikimedia.org/wikipedia/commons/a/a2/Elephants_Dream_%282006%29.webm", Path: "movies/Elephants Dream (2006).webm"},
	{URL: "https://download.blender.org/demo/movies/BBB/bbb_sunflower_2160p_60fps_normal.mp4.zip", Path: "movies/Big Buck Bunny (2008).mp4", ZipMember: "bbb_sunflower_2160p_60fps_normal.mp4"},

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
	{Library: "Demo - Films"},
	{Library: "Demo - Séries"},
	{Library: "Demo - Musique"},
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
// season 1 are watched, so Next up offers the following one.
type HistoryEntry struct {
	Film            string
	Percent         int
	Series          string
	WatchedEpisodes int
}

var History = []HistoryEntry{
	{Film: "Big Buck Bunny"},
	{Film: "Sintel", Percent: 40},
	{Film: "Spring", Percent: 60},
	{Series: "Colonel Bleep", WatchedEpisodes: 2},
	{Series: "Sherlock Holmes", WatchedEpisodes: 1},
}
