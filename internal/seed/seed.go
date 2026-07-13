// Package seed generates deterministic sample soldiers, records, and images into a DixieData data directory.
package seed

import (
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/debug"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
)

const (
	defaultSoldierCount = 250
	defaultSeed         = 1865
)

var (
	firstNames    = []string{"James", "William", "John", "Thomas", "Robert", "Samuel", "George", "Henry", "Joseph", "Charles", "Andrew", "Edward", "Benjamin", "Francis", "Nathaniel", "Lewis", "Richard", "Elijah", "Walter", "Jasper"}
	middleNames   = []string{"Allen", "Bell", "Clay", "Davis", "Edward", "Franklin", "Gray", "Henry", "Isaac", "Jasper", "Knox", "Lee", "Morgan", "Nathan", "Otis", "Perry", "Quincy", "Reuben", "Silas", "Thomas"}
	lastNames     = []string{"Carter", "Walker", "Hughes", "Bennett", "Foster", "McDaniel", "Pritchard", "Hawkins", "Turner", "Coleman", "Whitfield", "Mercer", "Dawson", "Reed", "Calhoun", "Harper", "Tate", "McBride", "Boone", "Abernathy"}
	ranks         = []string{"Private", "Corporal", "Sergeant", "Lieutenant", "Captain", "Major", "Colonel"}
	units         = []string{"1st Georgia Infantry", "4th Alabama Cavalry", "7th Texas Infantry", "12th Virginia Artillery", "15th Tennessee Infantry", "18th Mississippi Cavalry", "22nd North Carolina Infantry", "31st Louisiana Infantry", "3rd Arkansas Mounted Rifles", "5th South Carolina Infantry"}
	states        = []string{"Georgia", "Alabama", "Virginia", "Texas", "Mississippi", "Tennessee", "North Carolina", "South Carolina", "Louisiana", "Arkansas"}
	counties      = []string{"Madison County", "Jefferson County", "Franklin County", "Randolph County", "Monroe County", "Jackson County", "Warren County", "Marion County", "Lee County", "Greene County"}
	cemeteries    = []string{"Oakwood Cemetery, Richmond", "Magnolia Cemetery, Mobile", "Hollywood Cemetery, Richmond", "Elmwood Cemetery, Memphis", "Rose Hill Cemetery, Macon", "Confederate Rest, Helena", "Stonewall Cemetery, Winchester", "Greenwood Cemetery, New Orleans"}
	recordTypes   = []string{"Service Record", "Hospital Ledger", "Parole Note", "Pension Application", "Unit Roster", "Burial Ledger"}
	recordDetails = []string{
		"Filed from county records with marginal notes on service and discharge.",
		"Lists service dates, reported location, and clerk remarks preserved in the archive copy.",
		"Compiled summary prepared for local memorial indexing and cemetery cross-reference.",
		"Contains pension-era testimony transcribed into the registry abstract.",
		"Abstracted from adjutant returns and postwar veterans association notes.",
	}
	imageCaptions = []string{
		"Simulated portrait plate",
		"Simulated gravesite marker rubbing",
		"Simulated service card scan",
		"Simulated pension ledger excerpt",
		"Simulated regimental roster clipping",
	}

	// v58-v65 surface: event kinds, article titles, tag names for
	// the post-v58 entity seeding (issue #447).
	eventKinds = []string{"Battle", "Skirmish", "Siege", "Campaign", "Raid", "Muster"}
	eventDescs  = []string{
		"Engagement between opposing forces near the county seat. Casualty reports vary by source.",
		"Small-scale action involving local militia and a detached cavalry company.",
		"Fortification investment lasting several weeks; supply lines were cut early.",
		"Multi-week movement through contested territory with intermittent contact.",
		"Swift mounted operation targeting a supply depot behind enemy lines.",
		"Formal assembly of the regiment for inspection and payroll distribution.",
	}
	articleTitles = []string{
		"The Road to Manassas: A Prelude",
		"Camp Life in the Army of Northern Virginia",
		"Letters Home: A Soldier's Correspondence",
		"After the Surrender: Reconstruction in the South",
		"The Role of Cavalry in the Western Theater",
		"Walking the Line at Cemetery Ridge",
		"The Quartermaster's Ledger: Supplying a Regiment",
		"Women of the Confederacy: Voices from Home",
		"The Surgeons in the Field: Medicine Under Fire",
		"From Farm to Field: The Recruit's Journey",
		"The Cavalry Raid at Staunton River Bridge",
		"Blockade Running on the Atlantic Coast",
		"The Siege of Petersburg: A Soldier's Account",
		"Through the Wilderness: A Foot Cavalry March",
		"Desertion and Discipline in the Ranks",
		"The Freedmen's Bureau and the Aftermath",
		"Sharpshooters and Skirmishers: The Forgotten War",
		"Chaplains in Camp: Faith and Fatigue",
		"The Confederate Navy on the Inland Rivers",
		"Burial Details and the Dead of Winter",
		"Music and Morale: The Regimental Bands",
		"The Vicksburg Campaign: Crossing the Mississippi",
		"Railroads and Logistics of the Western Theater",
		"Union Prisoners at Andersonville: Survivor Testimony",
		"The Surrender at Appomattox: An Eyewitness",
		"Reconstruction-Era Veterans' Associations",
		"Mapping the Battle: Cartography in the Field",
		"Conscription and Class: The 1863 Draft Riots",
		"Civilians in the Crossfire: The Burning of Chambersburg",
		"Memory and Monuments: Early Memorial Campaigns",
	}
	articleBodies = []string{
		"In the spring of 1861, the gathering of forces along Bull Run marked the beginning of a long and bitter conflict. Soldiers from both sides arrived with high spirits and little understanding of what lay ahead.",
		"Daily life in camp revolved around drill, guard duty, and the constant search for adequate rations. Men wrote letters home describing the monotony punctuated by moments of sheer terror.",
		"My dearest Martha, I take up my pen this evening to tell you that I am well, though the march has been hard. We crossed the river at dawn and made camp in a pine grove.",
		"The years following the war brought hardship and hope in equal measure. Communities rebuilt, families reunited, and the long process of healing began.",
		"Mounted units played a decisive role in reconnaissance, screening, and raiding operations throughout the western campaigns. Their mobility often determined the outcome before infantry ever engaged.",
		"The peach orchard at the southern end of the line had been trampled by artillery the night before, and the smell of bruised fruit and powder hung over the field until midday. Our company held the fence line until the third wave broke against us.",
		"The quartermaster's ledger told the story of the war in a different voice: bolts of cloth, barrels of salt pork, tin cups issued and replaced, shoes worn out and returned for credit against the next allotment.",
		"Mothers, wives, and sisters kept the home fires burning and the correspondence flowing. Their letters, diaries, and petitions form a parallel record of the war that historians are only beginning to mine.",
		"The field hospital at Savage Station was a long tent with a packed-earth floor and straw pallets laid in rows. The surgeons worked through the night, and the orderlies kept the lanterns trimmed until their arms ached.",
		"He came off the farm in late summer with a homespun jacket, a musket he had never fired, and a letter from his mother sewn into the lining of his coat. He would not see the farm again for three years.",
		"The raiders approached the bridge at first light under cover of a thin ground fog. The signal was a single pistol shot; the charge was up the embankment and into the wagon yard before the pickets could form a line.",
		"The blockade runner slipped out of Wilmington on the tide, low in the water and dark-hulled, with a hold full of quinine and Enfield rifles. She made for Bermuda and did not see another Confederate port for two months.",
		"The trenches before Petersburg grew deeper with every passing week. By the second winter the men lived underground, and the only sound above the line was the picket shovel and the occasional sharpshooter's bullet.",
		"The march through the Wilderness was a kind of walking nightmare: the underbrush so thick the columns could not see one another, the smoke settling into the canopy and turning the sun to a dull red disc.",
		"The company records show a curious thing: the men who deserted were not always the cowards. Some had families starving at home. Some had lost faith in the cause. Some simply could not endure the next march.",
		"The Freedmen's Bureau arrived in the county seat the same week the last of the occupation troops departed. The agents worked out of a single borrowed room and tried to do the work of a government.",
		"The sharpshooter was a lonely trade. He went out at first light, found a perch in a tree or a chimney, and waited. He did not know the name of the man he might shoot, and the man he might shoot did not know his.",
		"The chaplain held services under a canvas awning rigged between two wagon tongues. The men sang hymns they remembered from childhood, and the sound carried across the camp and out into the dark.",
		"The riverboats of the Confederate Navy were improvised, half-armored, and crewed by men who had learned their trade on the inland streams. They fought ironclads with pine logs and plowshares.",
		"The burial detail worked by lantern light in the long freeze of January 1863. The ground was so hard the picks barely bit, and they sang a hymn to keep their hands moving.",
		"The regimental band practiced in the early morning when the air was cool and the brass was in tune. They played for dress parade, for funerals, and for the long evenings in camp when the men needed something to listen to other than the wind.",
		"The crossing at Bruinsburg caught the Union high command by surprise. The men waded ashore in the dark, their cartridge boxes held above their heads, and by dawn they had a foothold on the east bank.",
		"The railroads of the Western Theater were the arteries of the campaign. A locomotive off the schedule by twelve hours could mean the difference between a supply train and an empty siding.",
		"The survivor's account, written twenty years after the war, reads like a fever dream: the sun, the lice, the dead stacked along the fence, the single cup of water rationed at dawn.",
		"He rode to the McLean house on a damp morning with a small escort and a single valise. The terms were read, the signatures affixed, and the war ended in a parlor off the main road.",
		"The veterans' associations formed in almost every county in the decade after Appomattox. They held annual reunions, kept the muster rolls, and erected the monuments that still stand in courthouse squares.",
		"The map the colonel carried was hand-drawn by a civilian engineer who had never seen a battle. The lines were elegant. The terrain was wrong. The colonel used it anyway because it was the only map he had.",
		"The draft lottery was held in the county courthouse on a Saturday in July, and the crowd outside grew restless as the names were drawn. By nightfall the city was in flames and the conscription office was a smoking ruin.",
		"The civilians of Chambersburg watched the column approach from the west and knew there was no help coming. They gathered what they could carry and fled before the torches were lit.",
		"The memorial committees formed within a year of the surrender. They raised money by subscription, selected the sites, and contracted the stonecutters. The first monuments were standing before the ink was dry on the treaties.",
	}
	// Issue #447 vocabulary. Broad military career taxonomy so the
	// /tags page exercises a meaningful filter surface. Used as
	// both the tag inventory and the source for random tag
	// assignment on Person Records (seedPersonRecordTags).
	tagNames = []string{
		"Wounded", "POW", "Deserter", "Promoted", "Transferred",
		"KIA", "Died of Disease", "Paroled", "Enlisted", "Conscript",
		"Discharged", "Re-enlisted", "Missing in Action", "Captured",
		"Hospitalized", "Furloughed", "AWOL", "Court-Martialed",
		"Disabled", "Retired", "Color Bearer", "Sharpshooter",
		"Scout", "Courier", "Recruit", "Veteran", "Volunteer",
		"Substitute", "Mustered Out", "Detailed to Provost", "Survived the War",
	}

	// articleMarkdownBodies is the markdown corpus for the
	// --articles-format=markdown CLI surface (issue #523). Each
	// entry exercises a slice of the goldmark feature set so an
	// operator fixture round-trips through every renderer branch:
	// headings h1-h6, bold/italic/strikethrough, unordered +
	// ordered + nested lists, blockquote, inline code, fenced
	// code blocks, tables, links (including [[DXD-NNNNN]]
	// Person Record tokens), images with alt text, horizontal
	// rules. The Person Record tokens use out-of-range IDs
	// (DXD-999NN) so renderLinkedText falls back to literal text
	// without needing a real Person Record to resolve against.
	// Image URLs are public-domain Wikimedia Commons.
	articleMarkdownBodies = []string{
		// 1. Full kitchen-sink reference (every feature on one page).
		"# Markdown Feature Reference\n\nA working reference for every markdown syntax the DixieData article renderer supports.\n\n## Text Emphasis\n\nThis paragraph uses **bold**, *italic*, and ***bold italic*** together. A ~~strikethrough~~ shows the deletion marker. Inline `code` appears inside a sentence.\n\n> A blockquote spans multiple lines when the author continues the thought. The blockquote is its own block element.\n\n## Lists\n\nUnordered:\n\n- A private of the 4th Alabama\n- A corporal of the 7th Texas\n- A sergeant of the 12th Virginia\n\nOrdered:\n\n1. The regiment formed at dawn.\n2. The regiment marched by eight.\n3. The regiment engaged at noon.\n\nNested:\n\n- The brigade\n  - The 1st Georgia\n    - Company A\n    - Company B\n  - The 4th Alabama\n\n## Code\n\nInline code references like `fmt.Sprintf(\"DXD-%05d\", id)` render in monospace.\n\nFenced code block:\n\n```go\npackage main\n\nimport \"fmt\"\n\nfunc main() {\n    fmt.Println(\"Hello from a regiment's morning report\")\n}\n```\n\n## Tables\n\n| Rank       | Monthly Pay | Subsistence |\n| ---------- | ----------- | ----------- |\n| Private    | $11.00      | $4.00       |\n| Corporal   | $13.00      | $4.00       |\n| Sergeant   | $17.00      | $4.00       |\n| Lieutenant | $30.00      | $6.00       |\n\n| Quarter | Casualties |\n| ------: | ---------: |\n| Q1 1862 |         42 |\n| Q2 1862 |         73 |\n| Q3 1862 |         58 |\n\n## Links\n\nA plain link to [the National Park Service Civil War page](https://www.nps.gov/civilwar/) renders as a hyperlink.\n\nA link with a Person Record-style token: see [[DXD-99901]] for the referenced soldier.\n\n## Images\n\nAn image with alt text:\n\n![Confederate battle flag — public domain via Wikimedia Commons](https://upload.wikimedia.org/wikipedia/commons/thumb/1/1b/Confederate_National_Flag_since_March_4_1865.svg/320px-Confederate_National_Flag_since_March_4_1865.png)\n\n## Horizontal Rule\n\nAbove the rule.\n\n---\n\nBelow the rule.\n\n## Heading Levels\n\n### This is h3\n\n#### This is h4\n\n##### This is h5\n\n###### This is h6\n",
		// 2. Headings + table + blockquote
		"# Battle of Antietam: A Morning Report Excerpt\n\n> *September 17, 1862 — the bloodiest single day in American military history.*\n\n## Strength at Dawn\n\nThe brigade reported for duty at **first light** with the following strength:\n\n| Company | Present | Sick | Detached |\n| ------- | ------: | ---: | -------: |\n| A       |      38 |    2 |        1 |\n| B       |      41 |    0 |        2 |\n| C       |      36 |    3 |        0 |\n| D       |      39 |    1 |        1 |\n\nThe regimental total was **154 officers and men** of an authorized strength of 200.\n\n## The Engagement\n\n1. The regiment advanced through the **East Woods**.\n2. It paused at the *Dunkard Church* lane.\n3. It withdrew under heavy fire to the *west bank of the creek*.\n\nThe commanding officer's report closes:\n\n> We held our position from half-past six until ten o'clock, when, our ammunition being nearly exhausted and the supports failing to come up, we were obliged to retire in some confusion.\n\nSee [[DXD-99902]] for a soldier's service record from this engagement.\n",
		// 3. Bold/italic + ordered list + blockquote
		"# Letter from the Front\n\n*Camp near Manassas Junction, October 12, 1862*\n\n> Dear Mother,\n>\n> I take up my pen this evening to inform you that I am well, though the march has been hard. We crossed Bull Run on Tuesday last and made camp in a pine grove that the wind keeps whispering through.\n\n## The March\n\nWe have marched these distances since we left our last camp:\n\n- **Mon**: 14 miles — *Popes Creek to Dumfries*\n- **Tue**: 12 miles — *Dumfries to Bull Run*\n- **Wed**: 6 miles — *Bull Run to this place*\n\n## Rations\n\nThe commissary issued:\n\n1. Salt pork — 1 lb.\n2. Hardtack — 1 lb.\n3. Coffee — quarter pound\n4. Sugar — quarter pound\n\nThe hardtack is so hard we soak it in our coffee before we can bite it. There is talk of advancing on **Centreville** tomorrow. I shall write again on Sunday if the Lord spares me.\n\n> Your affectionate son,\n> *James*\n",
		// 4. Table + image + link
		"# The Surgeon's Tent at Savage Station\n\nThe field hospital at **Savage Station** was a long tent with a packed-earth floor and straw pallets laid in rows.\n\n## A Day's Intake\n\n| Hour  | Wounds Dressed | Amputations | Deaths |\n| ----- | -------------: | ----------: | -----: |\n| 06–09 |             14 |           0 |      0 |\n| 09–12 |             31 |           2 |      1 |\n| 12–15 |             28 |           3 |      2 |\n| 15–18 |             19 |           1 |      1 |\n\nThe surgeons worked through the night, and the orderlies kept the lanterns trimmed until their arms ached.\n\n> \"We had no chloroform for the last four cases. The men bore it as soldiers should.\" — *Asst. Surgeon H. Carter, 4th Alabama*\n\n![Field hospital layout — public domain via Wikimedia Commons](https://upload.wikimedia.org/wikipedia/commons/thumb/2/2f/Battle_of_Glendale_1862.png/320px-Battle_of_Glendale_1862.png)\n\nSee [[DXD-99904]] for a hospital ledger entry.\n",
		// 5. Table + bold/italic + ordered list
		"# Quartermaster's Ledger: A Week's Issues\n\nThe ledger told the story of the war in a different voice: bolts of cloth, barrels of salt pork, tin cups issued and replaced.\n\n## Issues by Day\n\n| Day   | Jackets | Pants | Shoes | Tin Cups |\n| ----- | ------: | ----: | ----: | -------: |\n| Mon   |       8 |    12 |     6 |       14 |\n| Tue   |      15 |    18 |     9 |       22 |\n| Wed   |       4 |     7 |     3 |        8 |\n| Thu   |      22 |    19 |    14 |       31 |\n| Fri   |      11 |    13 |     8 |       17 |\n| Sat   |       7 |     9 |     4 |       11 |\n\n## Totals\n\n1. Jackets — **67** issued\n2. Pants — **78** issued\n3. Shoes — **44** issued\n4. Tin cups — **103** issued\n\n> Note: the issue rate exceeded the intake rate by the close of the week. Recommend a foraging detail on Monday.\n",
		// 6. Headings + blockquote + table + nested list
		"# The Freedmen's Bureau in Madison County\n\nThe Bureau arrived in the county seat the same week the last of the occupation troops departed.\n\n## Office Locations\n\nThe Bureau established offices at the following sites:\n\n- **Court House** — main office\n- **Methodist Church** — school\n- *Old Tavern Building* — hospital\n- Freedmen's Bank branch — *corner of Main and Cherry*\n\n## Monthly Reports\n\n| Month   | Cases Opened | Cases Closed | Pending |\n| ------- | -----------: | -----------: | ------: |\n| Jan     |           42 |           31 |      11 |\n| Feb     |           38 |           35 |       3 |\n| Mar     |           51 |           47 |       4 |\n| Apr     |           29 |           26 |       3 |\n\n> The Bureau does not pretend to do the work of the state. It does, however, pretend to keep faith with the freedmen who look to it for redress. — *Report of the Assistant Commissioner*\n",
		// 7. Nested ordered list + bold/italic + blockquote
		"# Camp Life: A Soldier's Day\n\nDaily life in camp revolved around drill, guard duty, and the constant search for adequate rations.\n\n## Reveille\n\nThe drumbeat at **5:30 a.m.** sent the men stumbling into the cold morning air.\n\n### Schedule\n\n1. Reveille — 5:30\n2. Breakfast — 6:00\n   1. Salt pork\n   2. Hardtack\n   3. Coffee\n3. Drill — 7:00 to 9:00\n4. Fatigue duty — 9:00 to 11:00\n5. Drill — 2:00 to 4:00\n6. Dress parade — sunset\n7. Tattoo — 9:00\n8. Taps — 9:30\n\n## Fatigue Duties\n\n- Cutting firewood\n- Drawing water from the spring\n- Cleaning the company kitchen\n- Policing the company grounds\n\n> The men wrote letters home describing the monotony punctuated by moments of sheer terror.\n",
		// 8. Headings + table + blockquote + link
		"# The Cavalry Raid at Staunton River Bridge\n\nThe raiders approached the bridge at first light under cover of a thin ground fog.\n\n## Force Composition\n\n| Unit                  | Men  | Horses |\n| --------------------- | ---: | -----: |\n| 4th Alabama Cavalry   |   180 |    165 |\n| 12th Virginia Cavalry |   140 |    132 |\n| Horse artillery       |    42 |     38 |\n\n## The Signal\n\n> *A single pistol shot cracked the dawn.* The charge was up the embankment and into the wagon yard before the pickets could form a line.\n\n### Timeline\n\n- **05:42** — first shot, charge begins\n- **05:46** — wagon yard taken\n- **06:15** — bridge burned\n- **06:33** — withdrawal begins\n- **08:10** — re-cross at the ford\n\nSee [the Staunton River Bridge action on Wikipedia](https://en.wikipedia.org/wiki/Battle_of_Staunton_River_Bridge) for further context.\n",
		// 9. Table + bold/italic + ordered list + blockquote
		"# Blockade Running on the Atlantic Coast\n\nThe blockade runner slipped out of Wilmington on the tide, low in the water and dark-hulled, with a hold full of **quinine** and **Enfield rifles**.\n\n## Cargo Manifest\n\n| Item          | Quantity  | Origin       |\n| ------------- | --------: | ------------ |\n| Enfield rifles |     1,200 | Birmingham   |\n| Calico        |   400 bales | Manchester   |\n| Quinine       |     80 lbs | London       |\n| Lead (shot)   | 20 tons   | Liverpool    |\n| Coffee        |   150 sacks | Havana      |\n\n## Ports of Call\n\n1. *Wilmington* — outbound\n2. *Bermuda* — first call\n3. *Nassau* — second call\n4. *Havana* — final call\n\n> She made for Bermuda and did not see another Confederate port for two months.\n",
		// 10. Headings + table + ordered list + blockquote
		"# The Siege of Petersburg: A Soldier's Account\n\nThe trenches before Petersburg grew deeper with every passing week.\n\n## Daily Routine in the Trenches\n\n1. *Stand to arms* at dawn\n2. Breakfast — bacon and hardtack\n3. Fatigue — extending the parallels\n4. Dinner — salt pork and cornmeal\n5. Pickett — sunset\n6. Tour in the trench — first sleep rotation\n\n## Casualties by Week\n\n| Week    | Killed | Wounded | Sick |\n| ------- | -----: | ------: | ---: |\n| Jun 18  |      8 |      14 |   22 |\n| Jun 25  |      6 |      19 |   31 |\n| Jul  2  |      3 |      11 |   27 |\n| Jul  9  |     11 |      24 |   18 |\n\n> By the second winter the men lived underground, and the only sound above the line was the picket shovel and the occasional sharpshooter's bullet.\n",
		// 11. Bold/italic + table + ordered list + blockquote
		"# Through the Wilderness\n\nThe march through the Wilderness was a kind of walking nightmare.\n\n## Conditions\n\n- **Heat** — *95°F by midmorning*\n- **Smoke** — *settled into the canopy, sun a dull red disc*\n- **Underbrush** — *so thick the columns could not see one another*\n- **Dust** — *choked the column at every halt*\n\n## The Order of March\n\n| Division      | Position | Strength |\n| ------------- | -------- | -------: |\n| 1st Division  | Advance  |     4,200 |\n| 2nd Division  | Center   |     3,800 |\n| 3rd Division  | Rear     |     3,950 |\n| Reserve       | Follow   |     1,500 |\n\n> *\"Keep your files closed and your ranks dressed. We engage in twenty minutes.\"* — *Brigade Order, May 5, 1864*\n\nSee [[DXD-99911]] for a soldier's record.\n",
		// 12. Headings + nested lists + blockquote + table
		"# The Road to Manassas\n\nIn the spring of 1861, the gathering of forces along Bull Run marked the beginning of a long and bitter conflict.\n\n## The Gathering\n\nSoldiers from both sides arrived with high spirits and little understanding of what lay ahead.\n\n### From the North\n\n> \"The boys are in fine spirits. They think the war will be over in thirty days.\" — *NYT correspondent, July 1861*\n\n### From the South\n\n> \"We shall meet them and beat them in a single afternoon.\" — *Confederate private, 9th Virginia*\n\n## The Field\n\n| Approach       | Distance | Difficulty |\n| -------------- | -------: | ---------- |\n| Sudley Ford    |     6 mi | Easy       |\n| Stone Bridge   |     3 mi | Moderate   |\n| Blackburn's    |     8 mi | Difficult  |\n\nSee [[DXD-99912]] for a soldier's record.\n",
		// 13. Headings + table + blockquote
		"# The 1863 Draft Riots\n\nThe draft lottery was held in the county courthouse on a Saturday in July, and the crowd outside grew restless as the names were drawn.\n\n## The Lottery\n\n| Day    | Names Drawn | Substitutes | Exemptions |\n| ------ | ----------: | ----------: | ---------: |\n| Mon    |         412 |          17 |         28 |\n| Tue    |         388 |          22 |         41 |\n| Wed    |         401 |          14 |         35 |\n\n> By nightfall the city was in flames and the conscription office was a smoking ruin.\n\n## Casualties of the Riots\n\n- **Killed**: ~120\n- **Wounded**: ~2,000\n- **Property damage**: ~$5 million\n\nSee [[DXD-99913]] for an account from a New York regiment.\n",
		// 14. Table + bold/italic + ordered list + blockquote
		"# Memory and Monuments\n\nThe memorial committees formed within a year of the surrender.\n\n## Monument Sites\n\n| Site             | County      | Dedicated |\n| ---------------- | ----------- | --------: |\n| Courthouse Sq.   | Madison     |     1870  |\n| Magnolia Grove   | Jefferson   |     1871  |\n| Oakwood          | Henrico     |     1869  |\n| Stonewall        | Frederick   |     1872  |\n\n## Funding\n\nThe monuments were funded by:\n\n1. Veterans' associations — **40%**\n2. Public subscription — **35%**\n3. State appropriation — **25%**\n\n> They raised money by subscription, selected the sites, and contracted the stonecutters. The first monuments were standing before the ink was dry on the treaties.\n",
		// 15. Headings + table + blockquote + link
		"# The Sharpshooters of the 1st Confederate\n\nThe sharpshooter was a lonely trade.\n\n## Daily Report\n\n| Day  | Targets Engaged | Hits | Confirmed Kills |\n| ---: | --------------: | ---: | --------------: |\n|    1 |               7 |    3 |               2 |\n|    2 |               9 |    4 |               1 |\n|    3 |               5 |    2 |               1 |\n|    4 |              11 |    5 |               3 |\n|    5 |               8 |    4 |               2 |\n\n> *\"He went out at first light, found a perch in a tree or a chimney, and waited. He did not know the name of the man he might shoot, and the man he might shoot did not know his.\"*\n\nSee [Wikipedia's sharpshooter article](https://en.wikipedia.org/wiki/Sharpshooter) for the general concept.\n",
		// 16. Headings + table + ordered list + blockquote
		"# Burial Details\n\nThe burial detail worked by lantern light in the long freeze of January 1863.\n\n## A Day's Work\n\nThe detail consisted of **6 men** under the supervision of a sergeant.\n\n### Tasks\n\n1. Digging the trench — *frozen ground, picks barely biting*\n2. Wrapping the dead — *blankets cut from the regimental store*\n3. Recording the names — *pencil on paper, ink froze in the well*\n4. Reading the service — *the chaplain held a lantern*\n\n## Burials by Week\n\n| Week    | Buried |\n| ------- | -----: |\n| Jan  6  |     14 |\n| Jan 13  |      9 |\n| Jan 20  |     21 |\n| Jan 27  |     17 |\n\n> *\"The burial detail worked by lantern light and sang a hymn to keep their hands moving.\"*\n",
		// 17. Headings + table + bold/italic + blockquote
		"# Music and Morale\n\nThe regimental band practiced in the early morning when the air was cool and the brass was in tune.\n\n## Repertoire\n\nThe bandsmen could play:\n\n- **Dixie**\n- *The Bonnie Blue Flag*\n- **Maryland, My Maryland**\n- *Home Sweet Home*\n- **When Johnny Comes Marching Home**\n\n## Band Strength\n\n| Regiment    | Bandsmen | Instruments |\n| ----------- | -------: | ----------- |\n| 1st Georgia |       12 | brass + drum |\n| 4th Alabama |       10 | brass + drum |\n| 7th Texas   |       14 | full       |\n| 12th Va.    |        8 | fifes+drum |\n\n> *\"They played for dress parade, for funerals, and for the long evenings in camp when the men needed something to listen to other than the wind.\"*\n",
		// 18. Headings + table + bold + blockquote
		"# The Vicksburg Campaign\n\nThe crossing at **Bruinsburg** caught the Union high command by surprise.\n\n## The Crossing\n\nThe men waded ashore in the dark, their cartridge boxes held above their heads, and by dawn they had a foothold on the east bank.\n\n### Order of Crossing\n\n| Boat     | Troops       | Casualties |\n| -------- | ------------ | ---------: |\n| Lead     | 1st Brigade  |          3 |\n| Second   | 2nd Brigade  |          1 |\n| Third    | 3rd Brigade  |          5 |\n| Reserve  | Artillery    |          0 |\n\n## After the Crossing\n\n1. The brigade seized the heights above the landing.\n2. *Pemberton's pickets were driven in.*\n3. The march on **Vicksburg** began at noon.\n\n> *\"The navy had done its work. Now it was the army's turn.\"* — *Gen. Grant's report*\n",
		// 19. Table + blockquote + bold/italic
		"# Railroads and Logistics\n\nThe railroads of the Western Theater were the arteries of the campaign.\n\n## Key Lines\n\n| Route               | Length  | Gauge |\n| ------------------- | ------: | ----: |\n| Louisville–Nashville |    175 mi | std |\n| Chattanooga–Atlanta  |    119 mi | std |\n| Mobile–Montgomery    |    178 mi | std |\n| Memphis–Corinth      |    100 mi | std |\n\n## Schedule Discipline\n\n> *\"A locomotive off the schedule by twelve hours could mean the difference between a supply train and an empty siding.\"*\n\n### Critical Path\n\n1. **Nashville** — *supply depot*\n2. *Stevens Gap* — *junction*\n3. **Chattanooga** — *forward base*\n4. **Atlanta** — *objective*\n\nSee [[DXD-99919]] for a rail worker's record.\n",
		// 20. Headings + blockquote + table + strikethrough
		"# Andersonville: Survivor Testimony\n\nThe survivor's account, written twenty years after the war, reads like a fever dream.\n\n## Conditions\n\nThe summer of 1864 saw conditions that defied description:\n\n- **Sun** — *the heat rose to 110°F by noon*\n- **Lice** — *in every fold of every garment*\n- **Dead** — *stacked along the fence, sometimes unburied for days*\n- **Water** — *a single cup rationed at dawn*\n\n## The Dead Line\n\n> *\"The dead line was a fence post set into the ground at the inner edge of the stockade. Any man crossing it was shot without warning.\"*\n\n## Rations\n\n| Day     | Cornmeal | Salt Pork |\n| ------- | -------: | --------: |\n| Jul  1  |   1 lb   |  4 oz     |\n| Jul 15  |  10 oz   |  2 oz     |\n| Aug  1  |   8 oz   |  0 ~~oz~~  |\n\nSee [[DXD-99920]] for a prisoner's record.\n",
		// 21. Headings + table + blockquote
		"# The Surrender at Appomattox\n\nHe rode to the McLean house on a damp morning with a small escort and a single valise.\n\n## The Morning\n\n- **Date**: *April 9, 1865*\n- **Time**: *9:00 a.m.*\n- **Place**: *McLean House, Appomattox Court House*\n\n## Terms\n\n> *\"The terms were read, the signatures affixed, and the war ended in a parlor off the main road.\"*\n\n### Officers and Men\n\n| Status    | Officers | Men   |\n| --------- | -------: | ----: |\n| Paroled   |      800 | 28,000 |\n| To be returned | 200 |  3,400 |\n\n## Lee's Farewell\n\n> *\"After four years of arduous service, marked by unsurpassed courage and fortitude, the Army of Northern Virginia has been compelled to yield to overwhelming numbers and resources.\"* — *Gen. R. E. Lee, General Orders No. 9*\n\nSee [[DXD-99921]] for a soldier's parole record.\n",
		// 22. Table + ordered list + blockquote
		"# Veterans' Associations\n\nThe veterans' associations formed in almost every county in the decade after Appomattox.\n\n## Roll of Associations\n\n| Association        | County   | Members |\n| ------------------ | -------- | ------: |\n| 1st Georgia Vet.   | Madison  |     142 |\n| 4th Alabama Vet.   | Jefferson |     98 |\n| 7th Texas Vet.     | Bexar    |     115 |\n| 12th Va. Vet.      | Henrico  |     187 |\n\n## Annual Activities\n\n1. *Reunions* on the anniversary of the regiment's muster-in\n2. *Decoration Day* services at the local cemetery\n3. *Pension applications* filed jointly\n4. *Muster rolls* preserved at the courthouse\n\n> *\"They held annual reunions, kept the muster rolls, and erected the monuments that still stand in courthouse squares.\"*\n",
		// 23. Headings + blockquote + table
		"# Mapping the Battle\n\nThe map the colonel carried was hand-drawn by a civilian engineer who had never seen a battle.\n\n## The Map\n\n> *\"The lines were elegant. The terrain was wrong. The colonel used it anyway because it was the only map he had.\"*\n\n### Features Marked\n\n- **Roads** — *single and double track*\n- **Streams** — *fordable vs not*\n- **Bridges** — *capacity noted*\n- **Houses** — *named where known*\n\n### Errors of Note\n\n| Feature       | On Map      | In Reality  |\n| ------------- | ----------- | ----------- |\n| Stream depth  | 1 ft        | 4 ft        |\n| Road width    | 2 wagons    | 1 wagon     |\n| Bridge plank  | sound       | rotten      |\n\nSee [[DXD-99923]] for a surveyor's record.\n",
		// 24. Table + blockquote + ordered list
		"# Chambersburg Burning\n\nThe civilians of Chambersburg watched the column approach from the west and knew there was no help coming.\n\n## The Advance\n\n| Time    | Distance | Notes |\n| ------- | -------: | ----- |\n| 06:00   |        8 mi | *column at the foot of the ridge* |\n| 08:00   |        4 mi | *advance guard in sight* |\n| 09:30   |        0 mi | *column at the edge of town* |\n\n## The Demand\n\n> *\"Fifty thousand dollars in gold or your town burns.\"* — *Brig. Gen. McCausland's ultimatum*\n\n## The Destruction\n\n- **Public buildings** burned: **11**\n- *Private dwellings* burned: **278**\n- **Stores** burned: **42**\n\n> They gathered what they could carry and fled before the torches were lit.\n",
		// 25. Headings + table + blockquote
		"# Vicksburg's Surrender\n\nThe civilian population of Vicksburg endured **47 days** of siege before the garrison surrendered.\n\n## Rations During the Siege\n\n| Week | Cornmeal | Salt Pork | Molasses |\n| ---: | -------: | --------: | -------: |\n|   1  |   1 lb   |  1 lb     |   1 pt   |\n|   3  |  10 oz   |  6 oz     |  ½ pt    |\n|   5  |   4 oz   |  2 oz     |   0      |\n|   7  |   0      |  0        |   0      |\n\n> *\"The people ate mule meat and rice hulls before the surrender.\"* — *Diary of a Vicksburg resident*\n\nSee [[DXD-99925]] for a civilian's record.\n",
		// 26. Headings + ordered list + table + blockquote
		"# Memorial Day\n\nThe first observance was held in **Charleston, South Carolina**, on **May 1, 1865**.\n\n## Origins\n\n> *\"On May 1, 1865, formerly enslaved people, together with white missionaries and soldiers, gathered at the former race course in Charleston to honor the Union dead buried there.\"*\n\n## Observances by Year\n\n| Year | Place          | Attendance |\n| ---: | -------------- | ---------: |\n| 1865 | Charleston, SC |   ~10,000 |\n| 1866 | Arlington, VA  |    ~5,000 |\n| 1868 | Waterloo, NY   |    ~5,500 |\n\n## Traditions\n\n1. *Decoration* of graves with flowers\n2. **Reading** of the names of the fallen\n3. *Firing* of a salute\n4. **Bugler** plays *Taps*\n\nSee [[DXD-99926]] for a soldier's burial record.\n",
		// 27. Headings + ordered list + table + blockquote
		"# Reconstruction in the South\n\nThe years following the war brought hardship and hope in equal measure.\n\n## The Reconstruction Acts\n\n1. *Military Reconstruction Act* — **March 2, 1867**\n2. *Command of the Army Act* — **March 2, 1867**\n3. *Tenure of Office Act* — **March 2, 1867**\n4. *Second Reconstruction Act* — **March 23, 1867**\n\n## Voter Registration by State\n\n| State      | Black Reg. | White Reg. |\n| ---------- | ---------: | ---------: |\n| South Carolina | 80,000 |  46,000 |\n| Mississippi   | 60,000 |  46,000 |\n| Louisiana     | 45,000 |  48,000 |\n| Alabama       | 70,000 |  52,000 |\n\n> Communities rebuilt, families reunited, and the long process of healing began.\n",
		// 28. Headings + table + blockquote
		"# Freedmen's Bureau Schools\n\nThe Bureau established schools throughout the South to educate the newly freed population.\n\n## School Statistics\n\n| State       | Schools | Teachers | Pupils |\n| ----------- | ------: | -------: | -----: |\n| Virginia    |     174 |      210 |  9,400 |\n| N. Carolina |     132 |      156 |  7,100 |\n| S. Carolina |      98 |      118 |  5,200 |\n| Georgia     |     108 |      132 |  6,400 |\n| Alabama     |      94 |      110 |  4,900 |\n| Mississippi |     116 |      140 |  6,800 |\n\n> *\"The freedmen want to learn. They will sit for hours over a primer, tracing the letters with a finger on the page.\"* — *Teacher's report, Vicksburg*\n\nSee [[DXD-99928]] for a teacher's record.\n",
		// 29. Headings + table + blockquote + image
		"# Camp Douglas\n\n**Camp Douglas** in Chicago held Confederate prisoners from 1861 to the close of the war.\n\n## Population\n\n| Year  | Peak Population |\n| ----: | --------------: |\n|  1862 |             950 |\n|  1863 |           4,200 |\n|  1864 |           9,600 |\n|  1865 |           1,200 |\n\n## Mortality\n\nThe camp's mortality rate rose sharply during the **winter of 1863–64** due to:\n\n- *Cold* — *temperatures reached -20°F*\n- **Smallpox** outbreak — *one barrack lost 60 men in 30 days*\n- *Inadequate rations* — *issued at half scale*\n\n> *\"The dead were stacked like cordwood behind the hospital tent. We could not bury them fast enough.\"* — *Camp surgeon*\n\n![Camp Douglas prison stockade — public domain via Wikimedia Commons](https://upload.wikimedia.org/wikipedia/commons/thumb/9/9d/Camp_Douglas_Chicago.jpg/320px-Camp_Douglas_Chicago.jpg)\n\nSee [[DXD-99929]] for a prisoner's record.\n",
		// 30. Headings + table + ordered list + blockquote
		"# Pension Applications: A Clerk's Day\n\nThe pension clerk processed applications from sunrise to long past sundown.\n\n## Daily Throughput\n\n| Month   | Applications | Approved | Denied | Pending |\n| ------- | -----------: | -------: | -----: | ------: |\n| Jan     |          142 |       87 |     41 |      14 |\n| Feb     |          168 |       94 |     52 |      22 |\n| Mar     |          191 |      108 |     61 |      22 |\n| Apr     |          174 |       98 |     48 |      28 |\n\n## Forms\n\nEach application required:\n\n1. **Declaration** of service\n2. *Surgeon's certificate*\n3. **Witnesses** — *two fellow soldiers*\n4. *Property affidavit*\n\n> *\"The clerk's hand cramped by noon. By three, he had stopped reading the affidavits carefully.\"*\n",
	}
)

type Options struct {
	DataDir string
	Soldiers int
	Seed     int64
	Reset    bool
	// SkipSoldiers suppresses the soldier creation loop entirely.
	// Useful when the target Local Archive already has soldiers and
	// the caller wants to seed only the post-v58 entity surfaces
	// (Tags / Articles / Events). Pair with Tags/Articles/Events > 0
	// to add a fixture surface without touching existing soldier rows.
	SkipSoldiers bool
	// Tags caps how many of the tagNames vocabulary to insert. 0
	// (the default after normalizeOptions) means use the full
	// vocabulary length. Negative values are clamped to 0.
	Tags int
	// Articles caps how many Article rows the seeder inserts.
	// 0 (default) means use the existing 1-2 random selection.
	// Negative values are clamped to 0.
	Articles int
	// Events caps how many Event Record rows the seeder inserts.
	// 0 (default) means use the existing 20%-of-soldier count.
	// Negative values are clamped to 0.
	Events int
	// ArticlesFormat picks the article body source + render path.
	// ArticleBodyPlain (zero value, default) uses articleBodies
	// verbatim and wraps in <p>...</p> — the legacy #447 behavior.
	// ArticleBodyMarkdown uses articleMarkdownBodies and renders
	// body_md → body_html through records.MarkdownRenderer
	// (goldmark + bluemonday) so the fixture exercises the same
	// pipeline the Wails app uses on save. See issue #523.
	ArticlesFormat ArticleBodyFormat
}

// ArticleBodyFormat enumerates the article body render paths.
type ArticleBodyFormat int

const (
	// ArticleBodyPlain (default zero value) uses the legacy plain-
	// text corpus: body_md = prose, body_html = <p>{prose}</p>.
	ArticleBodyPlain ArticleBodyFormat = iota
	// ArticleBodyMarkdown uses the markdown corpus and renders
	// body_md → body_html via records.MarkdownRenderer.
	ArticleBodyMarkdown
)

// String satisfies fmt.Stringer for CLI flag validation.
func (f ArticleBodyFormat) String() string {
	switch f {
	case ArticleBodyMarkdown:
		return "markdown"
	default:
		return "plain"
	}
}

type Summary struct {
	DataDir  string
	DBPath   string
	ImageDir string
	Soldiers int
	Records  int
	Images   int
	// v58-v65 surface (issue #447)
	Events        int
	EventLinks    int
	EventSources  int
	Articles      int
	ArticleRefs   int
	Tags          int
	PersonRecordTags int
}

func Generate(options Options) (Summary, error) {
	options = normalizeOptions(options)
	if strings.TrimSpace(options.DataDir) == "" {
		return Summary{}, errors.New("data directory is required")
	}
	if !options.SkipSoldiers && options.Soldiers <= 0 {
		return Summary{}, errors.New("soldier count must be greater than zero (or set SkipSoldiers)")
	}

	dbPath := filepath.Join(options.DataDir, "dixiedata.db")
	imageDir := filepath.Join(options.DataDir, "images")

	if options.Reset {
		if err := resetData(dbPath, imageDir); err != nil {
			return Summary{}, err
		}
	}

	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		return Summary{}, fmt.Errorf("create image directory: %w", err)
	}

	database, err := db.Open(options.DataDir)
	if err != nil {
		return Summary{}, fmt.Errorf("open database: %w", err)
	}
	// Issue #449 slice 2: wrap the close in a closure so the
	// *DB.Close fires before Generate returns. The bare
	// `defer Close()` form defers the call to a returned
	// function value, which doesn't run until the enclosing
	// frame is gone — too late for the test caller that
	// immediately tries to RemoveAll the testtemp dir.
	defer func() { _ = database.Close() }()

	soldierSvc := records.NewSoldierService(database)
	conn := database.Conn()
	rng := rand.New(rand.NewSource(options.Seed))

	summary := Summary{
		DataDir:  options.DataDir,
		DBPath:   dbPath,
		ImageDir: imageDir,
	}

	if !options.SkipSoldiers {
		for i := 0; i < options.Soldiers; i++ {
			soldier := buildSoldier(rng, i)
		// Issue #377 slice 2: stamp the import path so future
		// "where did this row come from?" investigations can
		// attribute the row to the bulk seed importer. The
		// service-layer default already covers empty values, but
		// explicit stamping documents the intent at the call site
		// and survives any future defaulting change.
		soldier.CreatedByImportPath = "seed"
		created, err := soldierSvc.Create(soldier)
		if err != nil {
			return Summary{}, fmt.Errorf("create soldier %d: %w", i+1, err)
		}
		summary.Soldiers++

		recordCount := 1 + rng.Intn(3)
		for j := 0; j < recordCount; j++ {
			record := buildRecord(rng, *created, j)
			// Issue #447: stamp sort_order (form-array index)
			// so the v63 read path (ORDER BY sort_order, id)
			// is exercised.
			record.SortOrder = int64(j)
			if err := insertRecord(conn, *created, record); err != nil {
				return Summary{}, fmt.Errorf("create record for soldier %d: %w", created.ID, err)
			}
			summary.Records++
		}

		imageCount := 1 + rng.Intn(3)
		for j := 0; j < imageCount; j++ {
			image, err := createImage(options.DataDir, rng, *created, j)
			if err != nil {
				return Summary{}, fmt.Errorf("create image for soldier %d: %w", created.ID, err)
			}
			if err := insertImage(conn, *created, image); err != nil {
				return Summary{}, fmt.Errorf("insert image for soldier %d: %w", created.ID, err)
			}
			summary.Images++
		}
	}
}

	// Issue #447: seed v58-v65 surface — Event Records, Articles,
	// Tags, and their junction tables. Gated on schema version so
	// pre-v58 dev archives stay unaffected.
	var schemaVersion int
	if err := conn.QueryRow(`PRAGMA user_version`).Scan(&schemaVersion); err != nil {
		return Summary{}, fmt.Errorf("read schema version: %w", err)
	}
	if schemaVersion >= 58 {
		// Collect created soldier IDs for linking.
		soldierIDs, err := loadSoldierIDs(conn)
		if err != nil {
			return Summary{}, fmt.Errorf("load soldier IDs: %w", err)
		}

		// Tags: always seed the tag inventory. Person-record links
		// require at least one soldier to attach to.
		tagIDs, err := seedTags(conn, rng, options.Tags, &summary)
		if err != nil {
			return Summary{}, fmt.Errorf("seed tags: %w", err)
		}
		if len(soldierIDs) > 0 {
			if err := seedPersonRecordTags(conn, rng, soldierIDs, tagIDs, &summary); err != nil {
				return Summary{}, fmt.Errorf("seed person_record_tags: %w", err)
			}
		}

		// Event Records, links, sources, and Articles require at
		// least one soldier in the archive for the link tables to
		// resolve. Skip the v58-v65 entity surface entirely when the
		// archive has no soldiers — the caller can re-run after
		// seeding soldiers if they want events/articles.
		if len(soldierIDs) > 0 {
			// Event Records + links + sources.
			eventIDs, err := seedEvents(database, conn, rng, options.Events, &summary)
			if err != nil {
				return Summary{}, fmt.Errorf("seed events: %w", err)
			}
			if err := seedEventPersonLinks(conn, rng, eventIDs, soldierIDs, &summary); err != nil {
				return Summary{}, fmt.Errorf("seed event_person_links: %w", err)
			}
			if err := seedEventSources(conn, rng, eventIDs, &summary); err != nil {
				return Summary{}, fmt.Errorf("seed event_sources: %w", err)
			}
		}

		// Articles: the rows themselves don't depend on soldiers,
		// only the article_refs junction table does. Seed the
		// articles always (when --articles-format=markdown the
		// operator wants to exercise the goldmark pipeline even on
		// a soldier-less archive), then seed refs only when
		// soldiers exist.
		articleIDs, err := seedArticles(database, conn, rng, options.Articles, options.ArticlesFormat, &summary)
		if err != nil {
			return Summary{}, fmt.Errorf("seed articles: %w", err)
		}
		if len(soldierIDs) > 0 {
			if err := seedArticleRefs(conn, rng, articleIDs, soldierIDs, &summary); err != nil {
				return Summary{}, fmt.Errorf("seed article_refs: %w", err)
			}
		}
	}

	return summary, nil
}

func normalizeOptions(options Options) Options {
	if !options.SkipSoldiers && options.Soldiers == 0 {
		options.Soldiers = defaultSoldierCount
	}
	if options.Seed == 0 {
		options.Seed = defaultSeed
	}
	if options.Tags < 0 {
		options.Tags = 0
	}
	if options.Articles < 0 {
		options.Articles = 0
	}
	if options.Events < 0 {
		options.Events = 0
	}
	return options
}

func resetData(dbPath, imageDir string) error {
	if err := os.Remove(dbPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove existing database: %w", err)
	}
	for _, suffix := range []string{"-shm", "-wal"} {
		if err := os.Remove(dbPath + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove existing sqlite sidecar: %w", err)
		}
	}
	if err := os.RemoveAll(imageDir); err != nil {
		return fmt.Errorf("remove existing generated images: %w", err)
	}
	return nil
}

func buildSoldier(rng *rand.Rand, index int) models.Soldier {
	firstName := firstNames[rng.Intn(len(firstNames))]
	middleName := middleNames[rng.Intn(len(middleNames))]
	lastName := fmt.Sprintf("%s %s", lastNames[rng.Intn(len(lastNames))], string(rune('A'+(index%26))))
	state := states[rng.Intn(len(states))]
	county := counties[rng.Intn(len(counties))]
	month := 1 + rng.Intn(12)
	day := 1 + rng.Intn(28)
	rankInIndex := rng.Intn(len(ranks))
	rankOutIndex := rankInIndex + rng.Intn(len(ranks)-rankInIndex)
	rankIn := ranks[rankInIndex]
	rankOut := ranks[rankOutIndex]
	if rng.Intn(5) == 0 {
		day = 0
	}

	soldier := models.Soldier{
		PensionID:     fmt.Sprintf("P%05d", 10000+index),
		ApplicationID: fmt.Sprintf("A%05d", 10000+index),
		FirstName:     firstName,
		MiddleName:    middleName,
		LastName:      lastName,
		Rank:          rankOut,
		RankIn:        rankIn,
		RankOut:       rankOut,
		Unit:          units[rng.Intn(len(units))],
		PensionState:  state,
		DeathYear:     1861 + rng.Intn(5),
		DeathMonth:    month,
		DeathDay:      day,
		BirthInfo:     fmt.Sprintf("Born %d in %s, %s.", 1818+rng.Intn(25), county, state),
		BuriedIn:      cemeteries[rng.Intn(len(cemeteries))],
		Notes:         fmt.Sprintf("Generated test entry %03d for UI and export testing.", index+1),
	}

	return soldier
}

func buildRecord(rng *rand.Rand, soldier models.Soldier, index int) models.Record {
	recordType := recordTypes[rng.Intn(len(recordTypes))]
	detail := recordDetails[rng.Intn(len(recordDetails))]
	return models.Record{
		RecordType: recordType,
		AppID:      fmt.Sprintf("APP-%06d-%02d", soldier.ID, index+1),
		Details: fmt.Sprintf(
			"%s %s %s. %s",
			soldier.Rank,
			soldier.FirstName,
			soldier.LastName,
			detail,
		),
	}
}

func insertRecord(conn *sql.DB, soldier models.Soldier, record models.Record) error {
	syncID, err := db.NewSyncID()
	if err != nil {
		return err
	}
	// Issue #447: include sort_order (v63 read path) on every
	// seeded record. Provenance columns (created_by_version,
	// created_by_import_path) only exist on soldiers, not records.
	_, err = conn.Exec(
		`INSERT INTO records (sync_id, person_record_id, person_sync_id, record_type, app_id, details, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		syncID,
		soldier.ID,
		soldier.SyncID,
		record.RecordType,
		record.AppID,
		record.Details,
		record.SortOrder,
	)
	return err
}

func createImage(dataDir string, rng *rand.Rand, soldier models.Soldier, index int) (models.Image, error) {
	caption := imageCaptions[rng.Intn(len(imageCaptions))]
	recordDir, relativeDir := appdata.RecordImageDir(dataDir, soldier.DisplayID)
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		return models.Image{}, err
	}

	fileName := fmt.Sprintf("generated-%02d.png", index+1)
	filePath := filepath.Join(recordDir, fileName)

	img := image.NewRGBA(image.Rect(0, 0, 800, 500))
	base := color.RGBA{R: uint8(40 + rng.Intn(90)), G: uint8(30 + rng.Intn(50)), B: uint8(70 + rng.Intn(100)), A: 255}
	highlight := color.RGBA{R: uint8(180 + rng.Intn(60)), G: uint8(120 + rng.Intn(60)), B: uint8(60 + rng.Intn(40)), A: 255}
	shadow := color.RGBA{R: 20, G: 20, B: 35, A: 255}

	for y := 0; y < 500; y++ {
		for x := 0; x < 800; x++ {
			switch {
			case x > 40 && x < 760 && y > 40 && y < 460:
				img.Set(x, y, base)
			default:
				img.Set(x, y, shadow)
			}
			if (x/40+y/40)%5 == 0 && x > 80 && x < 720 && y > 80 && y < 420 {
				img.Set(x, y, highlight)
			}
		}
	}

	output, err := os.Create(filePath)
	if err != nil {
		return models.Image{}, err
	}
	defer func() { debug.DeferCloseLog(output, "createImage.output")() }()
	if err := png.Encode(output, img); err != nil {
		return models.Image{}, err
	}

	return models.Image{
		FileName: fileName,
		FilePath: filepath.Join(relativeDir, fileName),
		Caption:  caption,
	}, nil
}

func insertImage(conn *sql.DB, soldier models.Soldier, image models.Image) error {
	syncID, err := db.NewSyncID()
	if err != nil {
		return err
	}
	_, err = conn.Exec(
		`INSERT INTO images (sync_id, person_record_id, person_sync_id, file_name, file_path, caption) VALUES (?, ?, ?, ?, ?, ?)`,
		syncID,
		soldier.ID,
		soldier.SyncID,
		image.FileName,
		image.FilePath,
		image.Caption,
	)
	return err
}

// --- v58-v65 surface helpers (issue #447) ---

// loadSoldierIDs returns every soldier.id from the seeded archive
// so the event/article link helpers can pick random targets.
func loadSoldierIDs(conn *sql.DB) ([]int64, error) {
	rows, err := conn.Query(`SELECT id FROM soldiers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// seedTags inserts up to `count` rows from the tagNames vocabulary
// and returns their IDs. count <= 0 means insert the full vocabulary.
// Idempotent via INSERT OR IGNORE on normalized_name UNIQUE.
func seedTags(conn *sql.DB, rng *rand.Rand, count int, summary *Summary) ([]int64, error) {
	if count <= 0 || count > len(tagNames) {
		count = len(tagNames)
	}
	var ids []int64
	for _, name := range tagNames[:count] {
		res, err := conn.Exec(
			`INSERT OR IGNORE INTO tags (name, normalized_name) VALUES (?, ?)`,
			name, strings.ToLower(name),
		)
		if err != nil {
			return nil, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		// INSERT OR IGNORE returns 0 for existing rows; fetch the real ID.
		if id == 0 {
			if err := conn.QueryRow(`SELECT id FROM tags WHERE normalized_name = ?`, strings.ToLower(name)).Scan(&id); err != nil {
				return nil, err
			}
		}
		ids = append(ids, id)
		summary.Tags++
	}
	return ids, nil
}

// seedPersonRecordTags assigns 3-5 random tags to each soldier.
func seedPersonRecordTags(conn *sql.DB, rng *rand.Rand, soldierIDs, tagIDs []int64, summary *Summary) error {
	for _, sid := range soldierIDs {
		n := 3 + rng.Intn(3) // 3-5 tags per soldier
		// Shuffle tagIDs and pick the first n.
		perm := rng.Perm(len(tagIDs))
		for j := 0; j < n && j < len(perm); j++ {
			tid := tagIDs[perm[j]]
			if _, err := conn.Exec(
				`INSERT OR IGNORE INTO person_record_tags (person_id, tag_id) VALUES (?, ?)`,
				sid, tid,
			); err != nil {
				return err
			}
			summary.PersonRecordTags++
		}
	}
	return nil
}

// seedEvents creates N Event Record rows (entry_type='event') and
// returns their IDs. count == 0 means use the legacy default (~20%
// of soldier count, min 2). count < 0 is clamped to 0.
//
// Issue #537: display_id is minted via db.NextEventID() rather than a
// hard-coded `EVT-01000N` prefix so additive re-runs against an
// already-seeded archive don't collide on the unique constraint. The
// counter is namespace-scoped to EVT- and independent of the per-Person
// DXD- counter.
func seedEvents(database *db.DB, conn *sql.DB, rng *rand.Rand, count int, summary *Summary) ([]int64, error) {
	n := count
	if n == 0 {
		n = summary.Soldiers / 5
	}
	if n < 2 {
		n = 2
	}
	var ids []int64
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	for i := 0; i < n; i++ {
		kind := eventKinds[rng.Intn(len(eventKinds))]
		desc := eventDescs[rng.Intn(len(eventDescs))]
		month := 1 + rng.Intn(12)
		day := 1 + rng.Intn(28)
		year := 1861 + rng.Intn(5)
		beginDate := fmt.Sprintf("%02d/%02d/%d", month, day, year)
		endDate := fmt.Sprintf("%02d/%02d/%d", month, day+1+rng.Intn(3), year)
		syncID, err := db.NewSyncID()
		if err != nil {
			return nil, err
		}
		displayID, err := database.NextEventID()
		if err != nil {
			return nil, fmt.Errorf("mint event display_id: %w", err)
		}
		res, err := conn.Exec(
			`INSERT INTO soldiers (sync_id, display_id, entry_type, kind, begin_date, end_date, description, created_by_version, created_by_import_path, created_at, updated_at) VALUES (?, ?, 'event', ?, ?, ?, ?, ?, 'seed', ?, ?)`,
			syncID, displayID, kind, beginDate, endDate, desc, buildinfo.AppVersion, now, now,
		)
		if err != nil {
			return nil, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
		summary.Events++
	}
	return ids, nil
}

// seedEventPersonLinks attaches 1-3 random soldiers to each event.
func seedEventPersonLinks(conn *sql.DB, rng *rand.Rand, eventIDs, soldierIDs []int64, summary *Summary) error {
	for _, eid := range eventIDs {
		n := 1 + rng.Intn(3) // 1-3 soldiers per event
		perm := rng.Perm(len(soldierIDs))
		for j := 0; j < n && j < len(perm); j++ {
			sid := soldierIDs[perm[j]]
			syncID, err := db.NewSyncID()
			if err != nil {
				return err
			}
			if _, err := conn.Exec(
				`INSERT OR IGNORE INTO event_person_links (sync_id, event_id, person_id) VALUES (?, ?, ?)`,
				syncID, eid, sid,
			); err != nil {
				return err
			}
			summary.EventLinks++
		}
	}
	return nil
}

// seedEventSources creates 1-2 source records per event.
func seedEventSources(conn *sql.DB, rng *rand.Rand, eventIDs []int64, summary *Summary) error {
	sourceTypes := []string{"After-Action Report", "Casualty Return", "Morning Report", "Ordnance Return"}
	for _, eid := range eventIDs {
		n := 1 + rng.Intn(2) // 1-2 sources per event
		for j := 0; j < n; j++ {
			syncID, err := db.NewSyncID()
			if err != nil {
				return err
			}
			srcType := sourceTypes[rng.Intn(len(sourceTypes))]
			appID := fmt.Sprintf("SRC-%06d-%02d", eid, j+1)
			if _, err := conn.Exec(
				`INSERT INTO event_sources (sync_id, event_id, record_type, app_id, details, sort_order) VALUES (?, ?, ?, ?, ?, ?)`,
				syncID, eid, srcType, appID, recordDetails[rng.Intn(len(recordDetails))], j,
			); err != nil {
				return err
			}
			summary.EventSources++
		}
	}
	return nil
}

// seedArticles creates N Article rows and returns their IDs.
// count == 0 means use the legacy default (1-2 random). count < 0
// is clamped to 0. count > 0 uses that exact count.
//
// format selects the body source + render path:
//   - ArticleBodyPlain (default): legacy behavior — body_md is
//     raw prose from articleBodies, body_html wraps the same prose
//     in <p>...</p>. Matches the #447 path.
//   - ArticleBodyMarkdown: body_md is markdown source from
//     articleMarkdownBodies, body_html is the result of
//     records.NewMarkdownRenderer().Render(body_md) so the
//     fixture round-trips through the same goldmark +
//     bluemonday pipeline the Wails app uses on save. See #523.
// seedArticles creates N Article rows and returns their IDs. count == 0
// means use the legacy default of 1-2 articles.
//
// Issue #537: display_id is minted via db.NextArticleID() rather than
// a hard-coded `ART-01000N` prefix so additive re-runs against an
// already-seeded archive don't collide on the unique constraint. The
// counter is namespace-scoped to ART- and independent of both the
// per-Person DXD- counter and the per-Event EVT- counter.
func seedArticles(database *db.DB, conn *sql.DB, rng *rand.Rand, count int, format ArticleBodyFormat, summary *Summary) ([]int64, error) {
	n := count
	if n == 0 {
		n = 1 + rng.Intn(2) // 1-2 articles
	}
	var ids []int64
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	var renderer *records.MarkdownRenderer
	if format == ArticleBodyMarkdown {
		renderer = records.NewMarkdownRenderer()
	}
	for i := 0; i < n; i++ {
		var title, bodyMD, bodyHTML string
		switch format {
		case ArticleBodyMarkdown:
			title = articleTitles[rng.Intn(len(articleTitles))]
			bodyMD = articleMarkdownBodies[rng.Intn(len(articleMarkdownBodies))]
			rendered, err := renderer.Render(bodyMD)
			if err != nil {
				return nil, fmt.Errorf("markdown render for article %d: %w", i, err)
			}
			bodyHTML = rendered
		default:
			title = articleTitles[rng.Intn(len(articleTitles))]
			bodyMD = articleBodies[rng.Intn(len(articleBodies))]
			bodyHTML = "<p>" + bodyMD + "</p>"
		}
		syncID, err := db.NewSyncID()
		if err != nil {
			return nil, err
		}
		displayID, err := database.NextArticleID()
		if err != nil {
			return nil, fmt.Errorf("mint article display_id: %w", err)
		}
		res, err := conn.Exec(
			`INSERT INTO articles (sync_id, display_id, title, subtitle, body_md, body_html, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			syncID, displayID, title, "", bodyMD, bodyHTML, now, now,
		)
		if err != nil {
			return nil, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
		summary.Articles++
	}
	return ids, nil
}

// seedArticleRefs attaches 2-3 random soldiers to each article.
func seedArticleRefs(conn *sql.DB, rng *rand.Rand, articleIDs, soldierIDs []int64, summary *Summary) error {
	for _, aid := range articleIDs {
		n := 2 + rng.Intn(2) // 2-3 refs per article
		perm := rng.Perm(len(soldierIDs))
		for j := 0; j < n && j < len(perm); j++ {
			sid := soldierIDs[perm[j]]
			if _, err := conn.Exec(
				`INSERT OR IGNORE INTO article_refs (article_id, person_record_id, person_display_id, position) VALUES (?, ?, ?, ?)`,
				aid, sid, fmt.Sprintf("DXD-%05d", sid), j,
			); err != nil {
				return err
			}
			summary.ArticleRefs++
		}
	}
	return nil
}
