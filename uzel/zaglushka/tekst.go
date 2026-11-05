package zaglushka

// Тексты визиток. Сайт стоит на сервере в Европе, поэтому визитка
// европейская: тексты на английском, города и имена европейские. Мир это
// выдуманный владелец сайта: маленькая студия, мастерская или один человек,
// и кот у него талисман или персонаж. Внутри мира у каждой строки несколько
// вариантов, зерно выбирает по одному.
//
// В строках: %d год основания, %s имя кота, {г} город.

type usluga struct{ Nazvanie, Tekst string }

type mir struct {
	kod       string // только для тестов и подсчёта разнообразия, наружу не уходит
	imena     []string
	vid       []string
	zagolovki []string
	vvod      []string
	uslugi    []usluga
	oNas      []string
	podpisi   []string
	kontakt   []string
	knopki    []string
	gde       []string
	// Заголовки разделов: услуги, о нас, связь; и короткие пункты меню к ним
	razdely [3][]string
	menyu   [3]string
	pochta  []string
	palitry []palitra
	shrifty []shrift
}

var miry = []mir{
	{
		kod:       "animaciya",
		imena:     []string{"Springloaded", "Warm Frame", "Paper Elephant", "Keyframe & Co", "Slow Whale", "Six Paws", "Frame Twelve", "Inbetween Studio"},
		vid:       []string{"Character animation studio", "A small animation studio", "Animation and motion design"},
		zagolovki: []string{"We bring characters to life", "Characters that move like they mean it", "We draw heroes and teach them to move"},
		vvod: []string{
			"We design and animate characters for short films, games and title sequences. The cat on this page is dancing live in your browser. He is our test character.",
			"A small team: an animator, an illustrator and a director. We take on short projects and see them through to the last frame.",
			"We make short animated films and characters for brands, apps and games.",
		},
		uslugi: []usluga{
			{"Characters from scratch", "We find the personality, draw it and build a rig that is ready to animate."},
			{"Short films", "Clips under a minute for social media, presentations and intros."},
			{"Game animation", "Walk, jump and dance cycles exported for your engine."},
			{"Logo animation", "We bring your logo to life for the start and end of your videos."},
			{"Storyboards", "See how the film will look before animation starts."},
		},
		oNas: []string{
			"We have been animating since %d. We started with intros for friends and now make films for small companies and game studios.",
			"The studio opened in %d. We are a small team, so we work on every project ourselves, without subcontractors.",
		},
		podpisi: []string{"This is %s, our test character. Every new walk cycle gets tried on him first.", "Meet %s. He dances while we draw."},
		kontakt: []string{
			"Tell us what you would like to bring to life and send a couple of references. We reply within a day.",
			"Describe the project and the timeline, and we will suggest an approach and a quote.",
		},
		knopki:  []string{"Discuss a project", "Write to us"},
		gde:     []string{"{г}, working remotely across Europe.", "Based in {г}, working with clients everywhere."},
		razdely: [3][]string{{"What we do", "Services"}, {"About the studio", "Who we are"}, {"Get in touch", "Contact"}},
		menyu:   [3]string{"Services", "Studio", "Contact"},
		pochta:  []string{"hello", "studio", "mail"},
		palitry: []palitra{palNochnoyMontazh, palRaskadrovka},
		shrifty: []shrift{shGrotesk, shUzkiy},
	},
	{
		kod:       "illyustrator",
		imena:     []string{"Lena Hoffmann", "Mila de Vries", "Sanne Visser", "Clara Novak", "Jonas Berg", "Ines Moreau", "Tom Kessler", "Nora Lindqvist"},
		vid:       []string{"Illustrator", "Illustration and characters", "Drawing for books and games"},
		zagolovki: []string{"I draw characters with personality", "Illustration for books, games and packaging", "I draw animals you want to pet"},
		vvod: []string{
			"I am an illustrator. I draw for children's books, board games and packaging. The cat on this page is one of my characters too.",
			"I create characters and illustrations for publishers and small studios, from the first sketch to print-ready files.",
			"I love drawing animals and people with character. I take on books, games, packaging and stickers.",
		},
		uslugi: []usluga{
			{"Book illustration", "Covers and spreads for children's and adult titles."},
			{"Game characters", "Heroes, villains and pets with pose and expression sheets."},
			{"Packaging and labels", "Artwork that stands out on the shelf."},
			{"Sticker packs", "Sets for messengers based on your photos or ideas."},
			{"Pet portraits", "I draw your cat or dog from photos."},
		},
		oNas: []string{
			"I have been illustrating professionally since %d. I work remotely and agree on timelines and rounds of changes up front.",
			"Since %d I have worked with publishers and small studios. I take two or three projects at a time, so I can deliver on time.",
		},
		podpisi: []string{"%s, my main character and harshest critic.", "This is %s. He poses for free, but asks for treats."},
		kontakt: []string{
			"Tell me about the project, the timeline and the budget. I will reply with examples of relevant work.",
			"Describe your idea in a few words, and I will suggest how to draw it.",
		},
		knopki:  []string{"Write to me", "Commission a drawing"},
		gde:     []string{"{г}. Working remotely.", "Based in {г}, working with clients across Europe."},
		razdely: [3][]string{{"How I can help", "What I draw"}, {"About me", "A little about me"}, {"Get in touch", "Write to me"}},
		menyu:   [3]string{"Services", "About", "Contact"},
		pochta:  []string{"hello", "art", "mail"},
		palitry: []palitra{palShalfey, palGuash},
		shrifty: []shrift{shSerif, shGrotesk},
	},
	{
		kod:       "tancy",
		imena:     []string{"Little Steps", "Tiptoes", "Twirl", "Jump & Jive", "Pirouette Kids", "Carousel Dance", "Hop Skip", "Small Stage"},
		vid:       []string{"Dance studio for children", "Children's dance school", "Dance classes for ages 4 to 12"},
		zagolovki: []string{"Dancing from the age of four", "Dance classes they will ask for more of", "We teach children to dance, without tears or drills"},
		vvod: []string{
			"Classes for children aged 4 to 12: rhythm, ballet basics and contemporary dance. No more than ten children in a group.",
			"A small studio close to home. We teach children to hear the music, stand tall and enjoy the stage.",
			"Groups by age, classes twice a week and a recital every spring.",
		},
		uslugi: []usluga{
			{"Rhythm for little ones", "Ages 4 to 6: music, coordination and first combinations."},
			{"Ballet basics", "Ages 7 to 12: posture, stretching and dance combinations."},
			{"Contemporary dance", "Hip-hop and jazz funk for those who want to move faster."},
			{"Stage preparation", "Numbers for recitals and competitions, with help choosing costumes."},
			{"Trial class", "The first class is free, so your child can try it and decide."},
		},
		oNas: []string{
			"The studio opened in %d. Our teachers are trained dancers, and there are never more than ten children in a group.",
			"We have been dancing since %d. We have two mirrored halls and a waiting room for parents.",
		},
		podpisi: []string{"%s, the studio mascot. He knows every warm-up by heart.", "Meet %s, proof that you can dance at any age."},
		kontakt: []string{
			"Book a trial class, and we will find a group that suits your child's age and schedule.",
			"Tell us your child's age and the days that work for you, and we will suggest a group.",
		},
		knopki:  []string{"Book a trial class", "Find a group"},
		gde:     []string{"{г}. Classes by appointment.", "Classes in {г}, by appointment."},
		razdely: [3][]string{{"Classes", "What we teach"}, {"About the studio", "Who we are"}, {"Sign up", "How to sign up"}},
		menyu:   [3]string{"Classes", "Studio", "Sign up"},
		pochta:  []string{"hello", "dance", "info"},
		palitry: []palitra{palPodsolnukh, palMyata},
		shrifty: []shrift{shKruglyy, shGrotesk},
	},
	{
		kod:       "gostinica",
		imena:     []string{"Purr Inn", "The Warm Windowsill", "Whiskers & Paws", "Soft Paws Hotel", "Cat Harbour", "The Cat House", "Window Seat", "Velvet Paws"},
		vid:       []string{"Cat hotel", "Boarding for cats", "A hotel just for cats"},
		zagolovki: []string{"While you are away, your cat is on holiday too", "A hotel where cats feel at home", "Leave your cat with people who love cats"},
		vvod: []string{
			"Private rooms with shelves and a window, your cat's own feeding schedule and photos every day.",
			"We look after cats during holidays, moves and renovations. Cats only, no dogs and no noise.",
			"A small hotel with eight rooms. We know every guest by name.",
		},
		uslugi: []usluga{
			{"Room with a view", "Shelves at different heights, a cosy bed and a window onto the garden."},
			{"Their own routine", "We serve your cat's food at the usual times and give medication as prescribed."},
			{"Daily photos", "We send photos and videos, so you never miss a thing."},
			{"Grooming", "Brushing for long-haired cats and nail trims."},
			{"Pick-up service", "We collect your cat from home and bring them back."},
		},
		oNas: []string{
			"We opened in %d. We only accept vaccinated cats, and every room is cleaned and aired after each guest.",
			"We opened in %d, when we could not find a good place to leave our own cats. A vet is on call around the clock.",
		},
		podpisi: []string{"%s, the manager. He greets every guest and inspects the bowls.", "This is %s. He lives here and shows newcomers the best shelves."},
		kontakt: []string{
			"Send us your dates and a few words about your cat, and we will check which rooms are free.",
			"Book early: before the holidays our rooms are gone a month ahead.",
		},
		knopki:  []string{"Book a room", "Check availability"},
		gde:     []string{"{г}. Check-in by appointment.", "In {г}, check-in from 9:00 to 21:00."},
		razdely: [3][]string{{"What a stay includes", "Rooms and care"}, {"About the hotel", "Who we are"}, {"Booking", "Get in touch"}},
		menyu:   [3]string{"Rooms", "About", "Booking"},
		pochta:  []string{"hello", "booking", "info"},
		palitry: []palitra{palLavanda, palSumerki},
		shrifty: []shrift{shSerif, shKruglyy},
	},
	{
		kod:       "igry",
		imena:     []string{"Pixel Fox", "Two Paws Games", "Press Start", "Night Save", "Quiet Level", "Broken Joystick", "Level Three", "Small Hours Games"},
		vid:       []string{"A small game studio", "Independent game studio", "We make small games"},
		zagolovki: []string{"We are making a game about a dancing cat", "Small games for phones and computers", "Games you want to come back to"},
		vvod: []string{
			"We are three people: a programmer, an artist and a composer. Right now we are making a rhythm game, and its hero is dancing on this page.",
			"We make short games you can finish in an evening. We draw, compose and code everything ourselves.",
			"An independent studio of three. We have released two small games, and the third is in progress.",
		},
		uslugi: []usluga{
			{"A rhythm game about a cat", "Our new project: dance on the beat and win over the crowd."},
			{"Prototypes to order", "We test a game idea in two to three weeks."},
			{"Game animation", "Characters, effects and an interface that feels good to press."},
			{"Mobile ports", "We adapt controls and interfaces for touch screens."},
			{"Music and sound", "Soundtracks and sound effects that fit the mood of the game."},
		},
		oNas: []string{
			"We met at a game jam in %d and decided to keep going.",
			"We have been making games since %d. We work remotely and meet once a week to play our own builds.",
		},
		podpisi: []string{"%s, the hero of our next game. He dances to every beat.", "This is %s. In the game, he will dance to your music."},
		kontakt: []string{
			"We are looking for a publisher and playtesters. Write to us if you want to play before anyone else.",
			"Write to us about partnerships, prototypes and anything else to do with our games.",
		},
		knopki:  []string{"Contact us", "Become a playtester"},
		gde:     []string{"Working remotely, most of the team is in {г}.", "{г} and online."},
		razdely: [3][]string{{"What we are working on", "What we do"}, {"About the studio", "Who we are"}, {"Get in touch", "Contact"}},
		menyu:   [3]string{"Projects", "Studio", "Contact"},
		pochta:  []string{"hello", "team", "games"},
		palitry: []palitra{palSliva, palArkada},
		shrifty: []shrift{shMono, shUzkiy},
	},
	{
		kod:       "igrushki",
		imena:     []string{"Little Loop", "Yarn & Needles", "Woolly Yard", "Two Stitches", "Soft Shop", "Warm Paws", "Knitted Cat", "Purl & Plain"},
		vid:       []string{"Knitted toy workshop", "Handmade toys", "Knitted toys to order"},
		zagolovki: []string{"Knitted toys you will want to hug", "Handmade toys from your sketches", "Every toy is one of a kind"},
		vvod: []string{
			"We knit animals from cotton and merino. Choose one that is ready or order one based on a photo of your pet.",
			"A small workshop: two makers and a cat consultant. We make toys for children and gifts for grown-ups.",
			"Everything is knitted by hand, with no factory parts. Eyes are sewn on firmly, so the toys are safe for little ones.",
		},
		uslugi: []usluga{
			{"Toy from a photo", "We knit your cat or dog from photos."},
			{"Ready-made toys", "Animals, dolls and keyrings in stock, shipped the day you order."},
			{"Gift wrapping", "A box, a card and a message from you."},
			{"Workshops", "Learn to knit your first toy in two evenings."},
			{"Small things", "Keyrings, bookmarks and tree ornaments."},
		},
		oNas: []string{
			"The workshop opened in %d. We use hypoallergenic yarn and check every seam on every toy.",
			"We have been knitting since %d. We started with gifts for friends and now ship toys across Europe.",
		},
		podpisi: []string{"%s posed for our very first toy. These days he only dances.", "This is %s, our cat consultant. He checks every ball of yarn."},
		kontakt: []string{
			"Send us a photo and the size you would like, and we will quote a price and a date.",
			"Tell us who you would like knitted, and we will reply within a day.",
		},
		knopki:  []string{"Order a toy", "Ask for a price"},
		gde:     []string{"{г}. We ship across Europe.", "Workshop in {г}, shipping by post."},
		razdely: [3][]string{{"What we make", "What you can order"}, {"About the workshop", "Who we are"}, {"Order", "Get in touch"}},
		menyu:   [3]string{"Toys", "About", "Order"},
		pochta:  []string{"hello", "shop", "order"},
		palitry: []palitra{palPryazha, palLen},
		shrifty: []shrift{shKruglyy, shSerif},
	},
}

// Имена котов: подписи говорят о коте «he», имена к этому подходят
var imenaKotov = []string{
	"Biscuit", "Pepper", "Mochi", "Oscar", "Milo", "Ziggy", "Teddy", "Pumpkin",
	"Felix", "Toast", "Noodle", "Bruno", "Waffle", "Mango", "Leo",
}

var goroda = []string{
	"Berlin", "Hamburg", "Munich", "Cologne", "Leipzig", "Amsterdam", "Rotterdam", "Utrecht",
	"The Hague", "Antwerp", "Ghent", "Vienna", "Copenhagen", "Zurich", "Prague", "Lisbon",
}

var (
	podpisiPochty = []string{"Email:", "Write to", "Mail:"}

	oshibkaZagolovki = []string{"Page not found", "Nothing here", "This page does not exist"}
	oshibkaTeksty    = []string{
		"The address may be mistyped, or the page has been removed.",
		"Check the address or start from the home page.",
		"This link seems to be out of date.",
	}
	oshibkaSsylki = []string{"Go to the home page", "Back to home", "Home page"}
)
