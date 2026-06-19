package comicvine

import (
	"testing"
)

type parseTest struct {
	filename string
	series   string
	issue    string
	year     int
}

func TestParseFilename(t *testing.T) {
	tests := []parseTest{
		// standard filenames
		{"Amazing-Spider-Man 671 (2011) (Digital-Empire).cbr", "Amazing-Spider-Man", "671", 2011},
		{"American Vampire - Survival of The Fittest 05 (of 05) (2011) (digital-TheGroup).cbr", "American Vampire - Survival of The Fittest", "5", 2011},
		{"Baltimore - The Curse Bells 03 (of 05) (2011) (Minutemen-DTs).cbz", "Baltimore - The Curse Bells", "3", 2011},
		{"Batgirl 002 (2011) (digital-TheGroup).cbr", "Batgirl", "2", 2011},
		{"Buffy the Vampire Slayer Season 9 02 (2011) (two covers) (Minutemen-DTs).cbz", "Buffy the Vampire Slayer Season 9", "2", 2011},
		{"Comic Shop News 1269(c2c)(Comic Shop News)(2011)(YZ1).cbr", "Comic Shop News", "1269", 2011},
		{"Comic Shop News 1333.cbr", "Comic Shop News", "1333", 0},
		{"Critter 3(c2c)(2011)(Re-em).cbr", "Critter", "3", 2011},
		{"3 - Critter(c2c)(2011)(Re-em).cbr", "Critter", "3", 2011},
		{"3_Critter(c2c)(2011)(Re-em).cbr", "Critter", "3", 2011},
		{"Daily Bugle Avenging Spider-Man (2011) (digital-Empire).cbr", "Daily Bugle Avenging Spider-Man", "", 2011},
		{"Iron Man 2.0 006 (2011)(FBScan)(C2C).cbr", "Iron Man 2.0", "6", 2011},
		{"Iron Man 2.0 007.1 (2011)(FBScan)(C2C).cbr", "Iron Man 2.0", "7.1", 2011},
		{"Irredeemable 30 (2011) (two covers) (Minutemen-DTs).cbz", "Irredeemable", "30", 2011},
		{"Megaman 06 (2011) (Minutemen-DTs).cbz", "Megaman", "6", 2011},
		{"Red Hood & the Outlaws 001 (2011)(FBScan)(C2C).cbr", "Red Hood & the Outlaws", "1", 2011},
		{"S.H.I.E.L.D. 03 (2011) (c2c) (Team-CPS).cbr", "S.H.I.E.L.D.", "3", 2011},
		{"Zorro Rides Again! 003 (2011)(FBScan)(C2C).cbr", "Zorro Rides Again!", "3", 2011},
		{"Ultimate Vision 000 (2007) (c2c) (Popbot & ArtNet-DCP)", "Ultimate Vision", "0", 2007},
		{"Meta4 05 (of 05) (2011) (theProletariat-DCP)", "Meta4", "5", 2011},
		{"New York 5 03 (2011) (noads) (AngelicWezzy-CPS-DCP)", "New York 5", "3", 2011},
		{"I, Zombie 10 (2011) (fixed) (Minutemen-XxxX&Saxon)", "I, Zombie", "10", 2011},
		{"Draw 21.cbr", "Draw", "21", 0},
		{"Draw 21.0000000.cbr", "Draw", "21", 0},
		{"Draw -21.cbr", "Draw", "-21", 0},
		{"Batman Beyond V2010 #4 (2010).cbz", "Batman Beyond", "4", 2010},

		// page count patterns
		{"2000 AD 2000 (29-06-11) 295 pages.cbz", "2000 AD", "2000", 0},
		{"Ultimate Vision 000, 400p (2007) (c2c) (Popbot & ArtNet-DCP)", "Ultimate Vision", "0", 2007},
		{"Ultimate Vision 000, 400 pages, (2007) (c2c) (Popbot & ArtNet-DCP)", "Ultimate Vision", "0", 2007},
		{"Ultimate Vision 004a, 400pages (2007) (c2c) (Popbot & ArtNet-DCP).cbr", "Ultimate Vision", "4a", 2007},
		{"Ultimate Vision 000, 200 pgs. (2007) (c2c) (Popbot & ArtNet-DCP).cbr", "Ultimate Vision", "0", 2007},
		{"Ultimate_Vision_000_200pgs_(2007)_(c2c)_(Popbot & ArtNet-DCP).cbr", "Ultimate Vision", "0", 2007},
		{"I, Zombie 10 (2011) (200 pages) (fixed) (Minutemen-XxxX&Saxon).cbr", "I, Zombie", "10", 2011},
		{"I, Zombie 10 (2011) (400 pgs) (fixed) (Minutemen-XxxX&Saxon).cbr", "I, Zombie", "10", 2011},
		{"Supergirl v5 062 42pgs (2011) (c2c) (The Last Kryptonian-DCP).cbr", "Supergirl", "62", 2011},
		{"Supergirl v5 062 42 pgs. (2011) (c2c) (The Last Kryptonian-DCP).cbr", "Supergirl", "62", 2011},
		{"Supergirl_062_42pg_(2011)_(c2c)_(The Last Kryptonian-DCP).cbr", "Supergirl", "62", 2011},
		{"52 pages, Supergirl (2011)_(c2c)_(The Last Kryptonian-DCP).cbr", "Supergirl", "", 2011},
		{"Supergirl (2011), 40 pgs.cbr", "Supergirl", "", 2011},

		// weird bracketing
		{"Conan 0a [c2c][2011]  [Re-em].cbr", "Conan", "0a", 2011},
		{"Conan 0a {c2c } {2011}  {Re-em}.cbr", "Conan", "0a", 2011},
		{"Conan 0a [[c2c][2011]]  [Re-em].cbr", "Conan", "0a", 2011},
		{"Conan 0a (((c2c)(2011)  [Re-em])[special]).cbr", "Conan", "0a", 2011},

		// underscores
		{"S.H.I.E.L.D._03_(2011)_(c2c)_(Team-CPS).cbr", "S.H.I.E.L.D.", "3", 2011},
		{"2000AD_0001_(29-06-11).cbz", "2000AD", "1", 0},

		// series with numbers in name
		{"2000AD 1740 (29-06-11) (John Williams-DCP).cbr", "2000AD", "1740", 0},
		{"2000AD 0001 (29-06-11).cbz", "2000AD", "1", 0},
		{"2000AD 2000 (29-06-11).cbz", "2000AD", "2000", 0},
		{"2000 AD 2000 (29-06-11).cbz", "2000 AD", "2000", 0},
		{"2000 A.D. 2000 (29-06-11).cbz", "2000 A.D.", "2000", 0},
		{"2000A.D. 2011 (29-06-11).cbz", "2000A.D.", "2011", 0},
		{"Love Potion #9 5 (What?).cbz", "Love Potion #9", "5", 0},
		{"Love Potion #9, #5 (What?).cbz", "Love Potion #9", "5", 0},
		{"Love Potion #9 v300 4 (What?).cbz", "Love Potion #9", "4", 0},
		{"Love Potion #9.cbz", "Love Potion", "9", 0},
		{"5 Ronin 03 (2011) (Minutemen-DeliriousScanner)", "5 Ronin", "3", 2011},
		{"Scratch9 03 (of 04) (2010) (C2C) (Minutemen-DrSouse)", "Scratch9", "3", 2010},
		{"#3 Scratch9 (2010)", "Scratch9", "3", 2010},
		{"4 Scratch9 (2010)", "Scratch9", "4", 2010},
		{"4z Scratch9 (2010).cbr", "Scratch9", "4z", 2010},
		{"4.2 Scratch9 (2010).cbz", "Scratch9", "4.2", 2010},
		{"    4.3 Scratch9 (2010).cbz", "Scratch9", "4.3", 2010},

		// year-like issue numbers
		{"The Beano 1960.cbz", "The Beano", "1960", 0},
		{"The Beano 1960 V2010.cbz", "The Beano", "1960", 2010},
		{"The Beano #1960.cbz", "The Beano", "1960", 0},
		{"The Beano #1960 V2010.cbz", "The Beano", "1960", 2010},
		{"Cory Story 2010.cbz", "Cory Story 2010", "", 0},
		{"Cory Story #2010.cbz", "Cory Story", "2010", 0},
		{"Heavy Metal Magazine 197801 (1979).cbr", "Heavy Metal Magazine", "197801", 1979},
		{"Heavy Metal Magazine #197801 (1979).cbr", "Heavy Metal Magazine", "197801", 1979},

		// reading list prefixes
		{"001 The Zero-Men #132.cbz", "The Zero-Men", "132", 0},
		{"001 The Zero-Men #132.1 V2011 (2013).cbz", "The Zero-Men", "132.1", 2011},
		{"001 The Zero-Men #-132z.cbz", "The Zero-Men", "-132z", 0},
		{"002 Classic Zero-Men #038 - Story two (Zero-Men 132) (2014).cbr", "Classic Zero-Men", "38", 2014},
		{"002 Classic Zero-Men #038 - Story 2 (Zero-Men 132) [2011].cbr", "Classic Zero-Men", "38", 2011},
		{"003 Age of Apocalypse V2012 #3 (2014)", "Age of Apocalypse", "3", 2012},
		{"004 Age of Apocalypse #5 [2016]", "Age of Apocalypse", "5", 2016},

		// volume indicators
		{"Executive Assistant Iris V2 04 (2011) (Tarutaru-DCP).cbr", "Executive Assistant Iris", "4", 2011},
		{"Green Lantern v5 02 (2011) (c2c) (The Last Kryptonian-DCP).cbr", "Green Lantern", "2", 2011},
		{"Superboy v5 02 (2011) (c2c) (The Last Kryptonian-DCP).cbr", "Superboy", "2", 2011},
		{"Transformers v2 027 (2011) (2 covers) (GiantGreenGoat-DCP).cbr", "Transformers", "27", 2011},
		{"Supergirl v5 062 (2011) (c2c) (The Last Kryptonian-DCP)", "Supergirl", "62", 2011},

		{"Comic Shop News 1333 Vol. 3.cbr", "Comic Shop News", "1333", 0},
		{"Comic Shop News 1333 Vol.3.cbr", "Comic Shop News", "1333", 0},
		{"Comic Shop News 1333b V 3.cbr", "Comic Shop News", "1333b", 0},
		{"Comic Shop News -1333 Volume 3.cbr", "Comic Shop News", "-1333", 0},
		{"Comic Shop News -13.33 Vol 5.cbr", "Comic Shop News", "-13.33", 0},
		{"Comic Shop News -1.333c Vol5.cbr", "Comic Shop News", "-1.333c", 0},
		{"Comic Shop News 1333 V1.cbr", "Comic Shop News", "1333", 0},

		{"Comic Shop News Vol. 3 1222.cbr", "Comic Shop News", "1222", 0},
		{"Comic Shop News Vol.3 1222.cbr", "Comic Shop News", "1222", 0},
		{"Comic Shop News V 3 1222.cbr", "Comic Shop News", "1222", 0},
		{"Comic Shop News Volume 3a 1222.cbr", "Comic Shop News", "1222", 0},
		{"Comic Shop News Volume23 1222.cbr", "Comic Shop News", "1222", 0},
		{"Comic Shop News Vol 5cdf 1222.cbr", "Comic Shop News", "1222", 0},
		{"Comic Shop News Vol5s 1222cory.cbr", "Comic Shop News", "1222cory", 0},
		{"Comic Shop News V1 1222.cbr", "Comic Shop News", "1222", 0},

		{"Comic Shop News Vol. 3.cbr", "Comic Shop News", "", 0},
		{"Comic Shop News Vol.3.cbr", "Comic Shop News", "", 0},
		{"Comic Shop News V 3.cbr", "Comic Shop News", "", 0},
		{"Comic Shop News Volume 3.cbr", "Comic Shop News", "", 0},
		{"Comic Shop News Volume23.cbr", "Comic Shop News", "", 0},
		{"Comic Shop News Vol 5.cbr", "Comic Shop News", "", 0},
		{"Comic Shop News Vol5.cbr", "Comic Shop News", "", 0},
		{"Comic Shop News V1.cbr", "Comic Shop News", "", 0},

		{"Comic Shop News Vol..cbr", "Comic Shop News Vol.", "", 0},
		{"Comic Shop News V.cbr", "Comic Shop News V", "", 0},
		{"Comic Shop News Volume.cbr", "Comic Shop News Volume", "", 0},
		{"Comic Shop News Vol.cbr", "Comic Shop News Vol", "", 0},
		{"Comic Shop News Volumes 3.cbr", "Comic Shop News Volumes", "3", 0},

		// "of" problem
		{"Lone Ranger Death of Zorro 2 of 5 (2011)(FBScan)(C2C).cbr", "Lone Ranger Death of Zorro", "2", 2011},
		{"Powerman & Iron Fist 2of5 (2011)(FBScan)(C2C).cbr", "Powerman & Iron Fist", "2", 2011},
		{"Ultimate Comics Hawkeye 2 of4(2011)(FBScan)(C2C).cbr", "Ultimate Comics Hawkeye", "2", 2011},
		{"2 of 4 - Ultimate Comics Hawkeye (2011)(FBScan)(C2C).cbr", "Ultimate Comics Hawkeye", "2", 2011},

		// no issue number
		{"Conan - Free Comic Book Day 2006 Special V2006 #1 (2006).cbr", "Conan - Free Comic Book Day 2006 Special", "1", 2006},
		{"Marvel Season One Guide (c2c)(2011)(Re-em).cbr", "Marvel Season One Guide", "", 2011},
		{"Season One Guide (2011) (Digital) (Empire-Meganubis).cbr", "Season One Guide", "", 2011},
		{"The CBLDF Presents Liberty Annual 2011 (2011) (four covers) (c2c) (Minutemen-DTs).cbz", "The CBLDF Presents Liberty Annual 2011", "", 2011},
		{"Guff! (1998) (c2c) (FlipBook + Card) (Popbot & ArtNet-DCP)", "Guff!", "", 1998},
		{"Lorna, Relic Wrangler (2011) (Minutemen-DTs).cbz", "Lorna, Relic Wrangler", "", 2011},
		{"Red Sonja - Deluge (noads) (2011) (Minutemen-Penner).cbr", "Red Sonja - Deluge", "", 2011},

		// V-year format from ComicRack scripts
		{"Conan_ Road of Kings V2010 #3 (of 6) (2011)", "Conan Road of Kings", "3", 2010},
		{"King Conan_ The Scarlet Citadel V2011 #3 (of 4) (2011).zip", "King Conan The Scarlet Citadel", "3", 2011},
		{"Conan - Book of Thoth V2006 #2 (2006)", "Conan - Book of Thoth", "2", 2006},
		{"Conan the Cimmerian_ The Weight of the Crown V2010 #1 (2010).cbz", "Conan the Cimmerian The Weight of the Crown", "1", 2010},
		{"Star Wars_ The Old Republic - The Lost Suns V2011 #4 (of 5) (2011).cbz", "Star Wars The Old Republic - The Lost Suns", "4", 2011},
		{"Batman Beyond V2010 #3 (of 6) (2010).cbz", "Batman Beyond", "3", 2010},

		// embedded issue number mid-name
		{"Conan - Free Comic Book Day 2006 Special V2006 (2006).cbz", "Conan - Free Comic Book Day 2006 Special", "", 2006},
		{"Conan - Free Comic Book Day 2006a Special V2006 (2006).cbz", "Conan - Free Comic Book Day Special", "2006a", 2006},
		{"Cobra 04 - Cobra Civil War (2011)(noads)(Aardvark).cbr", "Cobra", "4", 2011},
		{"Cobra      05.3a - Cobra Civil War (2011)(noads)(Aardvark).cbr", "Cobra - Cobra Civil War", "5.3a", 2011},
		{"G.I. Joe v2 06 - Cobra Civil War (2011)(noads)(Aardvark).cbr", "G.I. Joe - Cobra Civil War", "6", 2011},
		{"Action Comics 200 V1 (1955-1)", "Action Comics", "200", 1955},

		// reading list number prefix
		{"013. Fear itself - The Worthy 02.cbr", "Fear itself - The Worthy", "2", 0},
		{"013 - Fear itself - The Worthy 02.cbr", "Fear itself - The Worthy", "2", 0},
		{" 13-Fear itself - The Worthy 02.cbr", "Fear itself - The Worthy", "2", 0},
		{"13.Fear itself - The Worthy 02.cbr", "13.Fear itself - The Worthy", "2", 0},
		{"13.3 - Fear itself - The Worthy 02.cbr", "13.3 - Fear itself - The Worthy", "2", 0},
		{"4 Conan.cbz", "Conan", "4", 0},
		{"4.2a Conan the Man.cbr", "Conan the Man", "4.2a", 0},

		// year extraction
		{"Conan is Cool (2006  ).cbz", "Conan is Cool", "", 2006},
		{"Conan is Cool [2006  ].cbz", "Conan is Cool", "", 2006},
		{"Conan is Cool 2002 ( 2006).cbz", "Conan is Cool 2002", "", 2006},
		{"Conan is Cool 2002 (2406).cbz", "Conan is Cool 2002", "", 0},
		{"Conan is Cool [2002 ] (2406).cbz", "Conan is Cool", "", 2002},
		{"Conan is Cool (1802) ( 2406).cbz", "Conan is Cool", "", 0},
		{"Conan is Cool (1802) (2009).cbz", "Conan is Cool", "", 2009},
		{"(2001)Conan is Cool (1802) (2109).cbz", "Conan is Cool", "", 2001},
		{"[2001]Conan is Cool (1802) (2109).cbz", "Conan is Cool", "", 2001},

		{"Conan is Cool (2006-2009 ).cbz", "Conan is Cool", "", 2006},
		{"Conan is Cool {2006 - 2009}.cbz", "Conan is Cool", "", 2006},
		{"Conan is Cool ( 2006 -09).cbz", "Conan is Cool", "", 2006},
		{"Conan is Cool (2006-9).cbz", "Conan is Cool", "", 2006},
		{"Conan is Cool [2006-9].cbz", "Conan is Cool", "", 2006},
		{"Conan is Cool {2006-9 }.cbz", "Conan is Cool", "", 2006},
		{"Conan is Cool [ 2006 - 9].cbz", "Conan is Cool", "", 2006},
		{"Conan is Cool (2006-229).cbz", "Conan is Cool", "", 2006},

		{"Conan is Cool #5 V2003.cbz", "Conan is Cool", "5", 2003},
		{"Conan is Cool #5 V2403.cbz", "Conan is Cool", "5", 0},
		{"Conan is Cool #5 V2003 (2005).cbz", "Conan is Cool", "5", 2003},
		{"Conan is Cool #5 (2008) V2003 (2005).cbz", "Conan is Cool", "5", 2003},
		{"V1995 Conan is Cool #5 V2003.cbz", "Conan is Cool", "5", 0},
		{"V1995 Conan is Cool #5.cbz", "Conan is Cool", "5", 1995},
		{"Conan is Cool #5 v2003", "Conan is Cool", "5", 2003},
		{"Conan is Cool_#5_v2003", "Conan is Cool", "5", 2003},
		{"Conan is Cool - #5 - v2003", "Conan is Cool", "5", 0},
		{"Conan is Cool-#5-v2003", "Conan is Cool", "5", 2003},
		{"Conan is Cool - v2003 - 5", "Conan is Cool", "5", 2003},
		{"Conan is Cool-v2003-#5", "Conan is Cool", "5", 2003},
		{"Conan is Cool, v2003, 5", "Conan is Cool", "5", 2003},
		{"Conan is Cool,v2003,#5", "Conan is Cool", "5", 2003},

		// embedded issue title
		{"Abe Sapien 001 - Dark and Terrible", "Abe Sapien", "1", 0},
		{"Star Wars Dark Times 33 -- The Fire Carrier (2013)(c2c)", "Star Wars Dark Times", "33", 2013},
		{"Breakout #012- This is it - run for it (noads)(2011)", "Breakout", "12", 2011},
		{"Buffy 55 - Can't stop the Signal (2015)", "Buffy", "55", 2015},

		// "v8" in series name (bug 364)
		{"DV8 #4", "DV8", "4", 0},
		{"DV8 28 V2007", "DV8", "28", 2007},
		{"#2 - Dv8 (2005).cbz", "Dv8", "2", 2005},

		// "of" in other languages
		{"The Filth #05 de #13", "The Filth", "5", 0},
		{"The Filth Too 5 de 10", "The Filth Too", "5", 0},
		{"The Filth Too #3de5", "The Filth Too", "3", 0},
		{"Italian Comic #0002 di #13", "Italian Comic", "2", 0},
		{"Italy Too! #12 di 10", "Italy Too!", "12", 0},
		{"Italy Too! 3de5", "Italy Too!", "3", 0},
		{"Max #00 von 13", "Max", "0", 0},
		{"Max Too 5 von 10 (2011)", "Max Too", "5", 2011},
		{"Max Too #13 von 5", "Max Too", "13", 0},
		{"The Filth #1 van 3", "The Filth", "1", 0},
		{"The Filth Too, 5 van 5", "The Filth Too", "5", 0},
		{"The Filth Too (ignore) #3 van5 (2003)", "The Filth Too", "3", 2003},
		{"The Witcher #2 z 15", "The Witcher", "2", 0},
		{"The Witcher 2 (noads)(2009), 5z5", "The Witcher 2", "5", 2009},
		{"The Witcher 3, #3 z 6  (2003)", "The Witcher 3", "3", 2003},

		// dashes
		{"4-Conan.cbz", "Conan", "4", 0},
		{"Conan-the-Man-32.cbr", "Conan the Man", "32", 0},
		{"--Conan-the-Man-32--.cbr", "Conan the Man", "32", 0},
		{"Conan-the-Man-#-32.cbr", "Conan the Man", "-32", 0},
		{"Conan-the-Man--32.cbr", "Conan the Man", "-32", 0},
		{"Conan_the_Man_-32.cbr", "Conan the Man", "-32", 0},
		{"Conan the Man -32.cbr", "Conan the Man", "-32", 0},

		// cover counts
		{"Batman 11 (02 of 3 covers).cbr", "Batman", "11", 0},
		{"2000 AD 3003 (01 of 44 covers).cbz", "2000 AD", "3003", 0},

		// pathological cases
		{"", "", "", 0},
		{"(this is a weird name).cbz", "(this is a weird name)", "", 0},
		{".weird name #5", ".weird name", "5", 0},
		{".werd name #2.cbz", ".werd name", "2", 0},
		{".", ".", "", 0},
		{"__", "__", "", 0},
		{"34", "34", "", 0},
		{"(what is this) 34.cbz", "(what is this) 34", "", 0},
		{"Fantastic Four 583-586 (2011) (Covers ONLY) (ScanDog & ArtNet-DCP)", "Fantastic Four", "583", 2011},
		{"Test Comic #4 (missing right bracket (cory).cbz", "Test Comic (missing right bracket", "4", 0},
	}

	for _, tc := range tests {
		t.Run(tc.filename, func(t *testing.T) {
			result := ParseFilename(tc.filename)
			if result.Series != tc.series {
				t.Errorf("Series: got %q, want %q", result.Series, tc.series)
			}
			if result.IssueNumber != tc.issue {
				t.Errorf("IssueNumber: got %q, want %q", result.IssueNumber, tc.issue)
			}
			if result.Year != tc.year {
				t.Errorf("Year: got %d, want %d", result.Year, tc.year)
			}
		})
	}
}

func TestParseFilenameWithRegex(t *testing.T) {
	result := ParseFilenameWithRegex(
		"Batman 042 (2015).cbz",
		`(?P<series>.+?)\s+(?P<num>\d+)\s+\((?P<year>\d{4})\)`,
	)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Series != "Batman" {
		t.Errorf("Series: got %q, want %q", result.Series, "Batman")
	}
	if result.IssueNumber != "042" {
		t.Errorf("IssueNumber: got %q, want %q", result.IssueNumber, "042")
	}
	if result.Year != 2015 {
		t.Errorf("Year: got %d, want %d", result.Year, 2015)
	}
}

func TestParseFilenameWithRegex_NoMatch(t *testing.T) {
	result := ParseFilenameWithRegex("random text", `(?P<series>NOMATCH)`)
	if result != nil {
		t.Errorf("expected nil for non-matching regex, got %+v", result)
	}
}

func TestParseFilenameWithRegex_InvalidRegex(t *testing.T) {
	result := ParseFilenameWithRegex("test.cbz", `[invalid`)
	if result != nil {
		t.Errorf("expected nil for invalid regex, got %+v", result)
	}
}

func TestIsYear(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"2000", true},
		{"1901", true},
		{"2099", true},
		{"1900", false},
		{"2100", false},
		{"999", false},
		{"20000", false},
		{"abcd", false},
	}
	for _, tc := range tests {
		if got := isYear(tc.input); got != tc.want {
			t.Errorf("isYear(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
