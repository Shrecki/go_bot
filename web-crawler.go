package main

import (
	//"fmt"
	"sync"
  "github.com/PuerkitoBio/goquery"
  "net/http"
  "log"
  "strings"
	"flag"
	"os"
	"time"
	"os/signal"
	"strconv"
	"sort"
	"unicode"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
	"golang.org/x/text/runes"
	"golang.org/x/net/html"
	"github.com/bwmarrin/discordgo"
	"regexp"
)

func removeAccents(s string) string  {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	output, _, e := transform.String(t, s)
	if e != nil {
		panic(e)
	}
	return output
}

func monitorWorker(wg *sync.WaitGroup, ch chan []string){
    wg.Wait()
    close(ch)
}

func camps_vers_emojis(s string) string{
	// Pour le moment on ne fait rien
	return s
}


func ExtracteurMission(wg *sync.WaitGroup, link string, ch chan PairString){
	defer wg.Done()
	res, err := http.Get(link)
	empty_pair := PairString{[]string{}, false}
	if err != nil {
		log.Fatal(err)
		ch <- empty_pair
	}

	flag := true;
	content := []string{};

	if res.Status != "200 OK"{
		log.Println("Request went awry:")
		log.Println(res.Status)
		flag = false;
	} else {
		doc, err := goquery.NewDocumentFromReader(res.Body)
		defer res.Body.Close()
		if err != nil {
			log.Fatal(err)
			ch <- empty_pair
		}
		productHTMLelement := doc.Find("#main-content").First();

		// On extrait le titre
		titre := strings.TrimSpace(productHTMLelement.Find(".topic-title").First().Text());


		// On doit trouver le premier message pour avoir la mission
		descr_mission := productHTMLelement.Find(".content").First();
		html_desc, err := goquery.OuterHtml(descr_mission);
		if err != nil {
			log.Fatal(err)
			ch <- empty_pair
		}

		// On extrait la difficulté
		difficulte_mission := strings.TrimSpace(descr_mission.Find(".captionFactionbox").First().Text());
		// On extrait les groupes
		//groupe_sep := ""
		groupes_mission := ""
		descr_rep := strings.ToLower(removeAccents(html_desc));

		if descr_rep == "" {
			log.Fatal("Description was empty, something went wrong")
		}

		diff_split := strings.Split(strings.Split(descr_rep, "difficulte")[1], "/");
		conv := string(diff_split[0][len(diff_split[0])-1]);
		res_conv, _ := strconv.Atoi(conv);

		emoji := ":star: ";
		if res_conv == 5 {
			emoji = ":star2: ";
		}

		num := strings.Repeat(emoji, res_conv);
		difficulte_mission = string(num);

		splits_potentiels := []string{"groupes concernes", "groupe concernes", "groupes concernes"}
		for i := range splits_potentiels {
			groupes := strings.Split(descr_rep, splits_potentiels[i])
			if len(groupes) > 1 {
				groupes_mission = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.Split(groupes[1], "<br/>")[0], "</span>", ""), ":", ""), "</div>", "");
				break;
			}
		}

		content = []string{link, titre, difficulte_mission, camps_vers_emojis(strings.Replace(groupes_mission, "&#39;", "'",1))}

	}
	ch <- PairString{content, flag};
}


type Pair struct {
	String_vals map[string][]string
	Get_ok bool
}

type PairString struct {
	Strings []string
	Get_ok bool
}

type PairHrefs struct {
	Hrefs []*html.Node
	Get_ok bool
}

func WalkPathsAndExtractContents(hrefs []*html.Node) Pair{
	wg := &sync.WaitGroup{}
	missions_web := make(map[string][]string)
	ch := make(chan PairString)

	flag  := true;
	for _, h := range hrefs {
		href := h.Attr[1].Val;
		wg.Add(1)
		go ExtracteurMission(wg, "https://la-force-vivante.forumsrpg.com" + href, ch)
	}

	// On demande aux groupes de patienter en matière de synchro
	go func(){
		wg.Wait();
		close(ch)
	}()
	for i := range ch {
			// On va aller chercher ici si quelque chose s'est mal passé. Si oui, on va mettre à false le flag!
			if i.Get_ok {
				curr_str := i.Strings
				missions_web[curr_str[0]] = curr_str[1:];
			} else {
				flag = false;
				break;
			}
	}
  return Pair{missions_web, flag}
}

// Only crawls to first level
func FirstLevelCrawl(url string) PairHrefs{
        //wg := &sync.WaitGroup{}
				//wg_discord := &sync.WaitGroup{}

        //missions_web := make(map[string][]string)
				// Create a single channel to be used within
				//ch := make(chan PairString)
				res, err := http.Get(url)
				//ch_discord := make(chan []string)
				var hrefs []*html.Node; // := nil;

				if err != nil {
					log.Fatal(err)
				}

				flag := true;

				if res.Status != "200 OK"{
					log.Println("Server side error:")
					log.Println(res.Status);
					flag = false;
				} else {
					doc, err := goquery.NewDocumentFromReader(res.Body)
					defer res.Body.Close()
					if err != nil {
						log.Fatal(err)
					}

					// Get all links in the page
					hrefs = doc.Find(".topictitle").Nodes;
				}
				return PairHrefs{hrefs, flag}
}

func init() { flag.Parse() }

var s *discordgo.Session
var guilds_watched map[string] string;
var BotID string;

var (
	GuildID        = flag.String("guild", "", "Test guild ID. If not passed - bot registers commands globally")
	BotToken       = flag.String("token", "", "Bot access token")
	RemoveCommands = flag.Bool("rmcmd", true, "Remove all commands after shutdowning or not")
)

func format_str(key string, values []string) string{
	return "[" + strings.Join(values, " ") + "](" + key + ") \n";
}

func format_compressed(titre string, values []string, url string) string{
	if values[1] == "" {
		log.Fatal("Cannot have empty value, ", values, " key", titre)
		return ""
	} else {
		//diff_str := strings.Split(values[1], "/")
		str :=  "**" + strings.Split(titre, "]")[0] + "]** *" + strings.TrimSpace(strings.Split(titre, "]")[2]) + "* " + values[1] + " " + values[2] + " :arrow_right: [Consulter]" + "("+ url + ")" +  "\n";
		//log.Println(str);
		return str;
	}
}

func conv_map_texte(missions map[string][]string) ([]string, []string, []string, []string, int, int, int, int){
	// First pass: count length
	/*length := 0
	msg_limit := 2000
	for key, values := range missions {
		length += len(format_str(key, values));
	}*/
	//s := make(map[string][]string);
	taches := make([]string, len(missions));
	primes := make([]string, len(missions));
	contrats := make([]string, len(missions));
	campagnes := make([]string, len(missions));
	n_camps := 0;
	n_primes := 0;
	n_contrats := 0;
	n_taches := 0;

	//s := 	make([]string,int(math.Ceil(float64(length)/float64(msg_limit))));
	//curr_window := 0
	log.Println("Conversion entamée")
	for url, values := range missions {
		//{link -> titre, difficulte_mission, camps}
		titre := values[0]
		if strings.Contains(titre, "Contrat"){
			contrats[n_contrats] = format_compressed(titre, values, url);
			n_contrats += 1;
		} else if strings.Contains(titre, "Tâche"){
				taches[n_taches] = format_compressed(titre, values, url);
				n_taches +=1;
		} else if  strings.Contains(titre, "Prime"){
					primes[n_primes] = format_compressed(titre, values, url);
					n_primes += 1;
			} else if strings.Contains(titre, "Campagne"){
				campagnes[n_camps] = format_compressed(titre, values, url);
				n_camps += 1;
			} else {
				log.Fatalf("Type de mission non reconnu. Doit être Contrat, Tâche, Prime ou Campagne:", titre);
			}
		}

		// Une fois les différentes cartes constituées, on les retourne tout simplement
	return taches, primes, contrats, campagnes, n_taches, n_primes, n_contrats, n_camps;
}

func convertListToString(list_name string, list_content []string, n_elems int) string{
	s := ""
	s += list_name + "\n"
	//log.Println(list_content[0:n_elems])
	s += strings.Join(list_content[0:n_elems], "")
	return s;
}

func convertToSingleString(liste_campagne []string, liste_contrats []string, liste_primes []string, liste_taches[]string,
	n_campagnes int, n_contrats int, n_primes int, n_taches int) string{
	s := "# Missions \n"
	s += convertListToString("## Campagnes", liste_campagne, n_campagnes);
	s += convertListToString("## Contrats", liste_contrats, n_contrats);
	s += convertListToString("## Primes", liste_primes, n_primes);
	s += convertListToString("## Tâches", liste_taches, n_taches);
	return s;
}

func init() {
	// Start the bot_name
	var err error
	s, err = discordgo.New("Bot " + *BotToken)
	if err != nil {
		log.Fatalf("Invalid bot parameters : %v", err)
	}
}

func SendMessageNoEmbed(s *discordgo.Session, chan_id string, content string) (*discordgo.Message, error){
	return s.ChannelMessageSendComplex(chan_id, &discordgo.MessageSend{
		Content: content,
		Flags: discordgo.MessageFlagsSuppressEmbeds, // Get latest discordgo version to have the flags. Build it from source.
	})
}

func emit_response(s *discordgo.Session, string_to_send string, chan_id string){
	if len(string_to_send) > 2000 {
		// Split in sub-messages. We first create these substrings and then send them one by one to ensure consistent ordering.
		split_str := strings.Split(string_to_send, "\n");
		msg_contents := make([]string, len(split_str));
		n_words := 0
		curr_start := 0
		curr_msg := 0
		for i := range split_str {
			if len(split_str[i]) + n_words < 2000 {
				// Reached last line and need to send it
				if i == len(split_str) -1 {
					msg_contents[curr_msg] = strings.Join(split_str[curr_start:i], "\n")
					//SendMessageNoEmbed(s, chan_id, );
					curr_msg++;
				} else {
					n_words += len(split_str[i])
				}
			} else {
				msg_contents[curr_msg] = strings.Join(split_str[curr_start:i], "\n");
				curr_msg++;
				//SendMessageNoEmbed(s, chan_id, strings.Join(split_str[curr_start:i], "\n"));
				curr_start = i;
				n_words = len(split_str[i]);
			}
		}

		// Now, we will send each message and wait until the message is done to send another one.
		for msg_i := range msg_contents[:curr_msg] {
			_, err := SendMessageNoEmbed(s, chan_id, msg_contents[msg_i]);
			if err != nil {
				log.Println("Something happened during message transmission")
				log.Println(err);
			}
		}
	} else {
		SendMessageNoEmbed(s, chan_id, string_to_send);
	}
}

func filter_bot_messages(msgs []*discordgo.Message) []*discordgo.Message{
	bot_msgs := make([]*discordgo.Message, len(msgs))
	n_messages := 0

	if len(msgs) > 0 {
		// Si messages présents: on récupère le contenu DANS L'ORDRE DE MESSAGES CROISSANTS.
		// On va donc devoir les trier par timestamp
		sort.SliceStable(msgs, func(i, j int) bool {
			return msgs[i].ID < msgs[j].ID
		});

		for i := range msgs {
			str_author := msgs[i].Author
			// On ne considère que les messages du bot
			if str_author.ID == s.State.User.ID {
				log.Println("I found messages I own")
				bot_msgs[n_messages] = msgs[i]
				n_messages ++;
			}
		}
	}

	return bot_msgs[:n_messages];

}

/**
* Renvoie les URLs dans les messages du bots sous la forme d'une liste
**/
func get_bot_message_URLs(msgs []*discordgo.Message) []string{
	// On a quelque chose à faire!
	url_list := make([]string, 200);
	n_matches := 0
	if len(msgs) > 0 {
		// Si messages présents: on récupère le contenu DANS L'ORDRE DE MESSAGES CROISSANTS.
		// On va donc devoir les trier par timestamp
		sort.SliceStable(msgs, func(i, j int) bool {
			return msgs[i].ID < msgs[j].ID
		});

		str_content := ""
		for i := range msgs {
				str_content += msgs[i].Content + "\n";
		}

		// On construit une liste d'urls, en fonction du nombre de messages. Rien de plus simple!
		re := regexp.MustCompile("\\(https.*\\)")
    url_matches := re.FindAllString(str_content, -1)
		n_matches = len(url_matches);
		//url_list := make([]string, len(url_matches))
		for i, match := range url_matches {
			url_list[i] = match[1:len(match)-1];
		}

	}

	return url_list[:n_matches];

}


func check_and_update_missions(chan_id string) bool{
	out1 := make(chan PairHrefs)
	out2 := make(chan []*discordgo.Message)
	out3 := make(chan Pair)
	out4 := make(chan []string)
	out5 := make(chan []string)


	go func() {
			out1 <- FirstLevelCrawl("https://la-force-vivante.forumsrpg.com/f21-missions-disponibles");
	}();
	go func() {
			msgs, _ := s.ChannelMessages(chan_id, 100, "", "", "")
			// For each message filter to retain only those belonging to the bot
			out2 <- msgs
	}();
	// On doit aller chercher les missions sur le forum et les garder dans leur map.
	res := <- out1;
	hrefs_online := res.Hrefs;
	increase_wait := false;
	if !res.Get_ok {
		log.Println("First page fetch failed. Increasing timer.")
		increase_wait = true;
	} else {
		msgs := <- out2;
		// On va regarder le contenu des hrefs. Pour ceci, on va prendre dans les messages Discord
		// tous les URLs disponibles. On vérifie que les deux listes d'URLs ont la même taille.
		// Si ce n'est pas le cas, soit une mission n'est plus dispo, soit une nouvelle mission est dispo: il faut mettre à jour
		// Sinon, on doit comparer les URLs entre eux. On peut donc tout simplement construire une map et vérifier l'égalité des deux.
		go func(){
			filt_msgs := filter_bot_messages(msgs);
			url_list := get_bot_message_URLs(filt_msgs);
			msg_ids := make([]string, len(filt_msgs))
			// Keep only message IDs!
			for i := range filt_msgs {
				msg_ids[i] = filt_msgs[i].ID
			}
			out4 <- url_list
			out5 <- msg_ids
		}()

		//

		different := false
		bot_url_list := <- out4;
		msg_ids := <- out5;

		if len(hrefs_online) == len(bot_url_list) {
			// Compare the hrefs one by one to check potential diffs: create a small url map
			url_map := make(map[string]int)
			for _, url_bot := range bot_url_list {
				url_bot_split := strings.Split(url_bot, "https://la-force-vivante.forumsrpg.com")[1]
				url_map[url_bot_split] = 1;
			}

			for _, h := range hrefs_online {
				url_online :=  h.Attr[1].Val
				_, ok := url_map[url_online];
				// Une URL du forum n'est pas sur le Discord!
				if !ok {
					different = true;
					break;
				}
			}
		} else {
			different = true;
		}

		// Si le contenu diffère, on doit aller chercher le contenu en ligne pour mettre à jour le Discord.
		// Il faut donc supprimer *tous* les messages du bot sur le channel et régénérer simultanément le message à envoyer.
		if different {
			// On doit aller chercher le contenu des pages et générer un nouveau message.
			log.Println("Preparing messages to be sent.")

			// On extrait ici les pages sous forme de liste
			go func(){
				out3 <- WalkPathsAndExtractContents(hrefs_online);
			}()
			res := <- out3;
			missions_extraites := res.String_vals;
			ok := res.Get_ok;
			if !ok {
				increase_wait = true;
			} else {
				taches, primes, contrats, campagnes, n_taches, n_primes, n_contrats, n_campagnes := conv_map_texte(missions_extraites);
				string_to_send := convertToSingleString(campagnes, contrats, primes, taches, n_campagnes, n_contrats, n_primes, n_taches);

				// On supprime tous les messages du bot
				s.ChannelMessagesBulkDelete(chan_id, msg_ids)

				// On émet la réponse
				emit_response(s, string_to_send, chan_id)
			}
		}
	}

	return increase_wait;

}

func watchdog(original_delay float64, quit chan struct{}, chan_id string) {
		ticker := time.NewTicker(time.Duration(original_delay) * time.Second)
		current_delay := original_delay;
    for {
       select {
        case <- ticker.C:
					  log.Println("Querying server...")
						increase_wait := check_and_update_missions(chan_id);
						if increase_wait {
							current_delay = current_delay * 2;
							ticker.Stop()
							ticker = time.NewTicker(time.Second * time.Duration(current_delay));
						} else {
							ticker.Stop()
							ticker = time.NewTicker(time.Duration(original_delay) * time.Second);
						}
        case <- quit:
            ticker.Stop()
            return
        }
    }
 }

var(
	integerOptionMinValue          = 1.0
	dmPermission                   = false
	defaultMemberPermissions int64 = discordgo.PermissionAllText
	commands = []*discordgo.ApplicationCommand{
		{
			Name: "liste-missions",
			// All commands and options must have a description
			// Commands/options without description will fail the registration
			// of the command.
			Description: "Liste les missions disponibles actuellement",
		},
	}
		commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
			"liste-missions": func(s *discordgo.Session, i *discordgo.InteractionCreate) {

				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Processing...",
					},
				})
				// Get on one side the first level. On the other side get the channel messages
				chan_id := i.Interaction.ChannelID

				// On vérifie si le serveur est déjà surveillé. Si oui, on appelle simplement la fonction. Sinon, on lance aussi un thread de watcher
				// On récupère les URLs des missions

				// On récupère les messages Discord

				// On compare les URL dans les messages et les URLs online. S
				check_and_update_missions(chan_id);
				if _, ok := guilds_watched[i.Interaction.GuildID]; !ok {
					// On crée une nouvelle queue et un nouveau Ticker
					quit := make(chan struct{})
					// La durée est en secondes. On souhaite un rafraîchissement toutes les 10 minutes, soit toutes les 600 secondes
					go watchdog(600, quit, chan_id);
				} else {
					guilds_watched[i.Interaction.GuildID] = "watched";
				}
				s.InteractionResponseDelete(i.Interaction);
			},
		}
	)

func init() {
		s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
				h(s, i)
			}
		})
}

func main(){
	  s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
			log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
		})

		err := s.Open()
		if err != nil {
			log.Fatalf("Cannot open the session: %v", err)
		}

		BotID = s.State.User.ID;
		log.Println("My ID is %v", BotID);

		log.Println("Adding commands...")
		registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
		for i, v := range commands {
			cmd, err := s.ApplicationCommandCreate(BotID, *GuildID, v)
			if err != nil {
				log.Panicf("Cannot create '%v' command: %v", v.Name, err)
			}
			registeredCommands[i] = cmd
		}

		defer s.Close()

		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt)
		log.Println("Press Ctrl+C to exit")
		<-stop

		if *RemoveCommands {
			log.Println("Removing commands...")
			// // We need to fetch the commands, since deleting requires the command ID.
			// // We are doing this from the returned commands on line 375, because using
			// // this will delete all the commands, which might not be desirable, so we
			// // are deleting only the commands that we added.
			// registeredCommands, err := s.ApplicationCommands(s.State.User.ID, *GuildID)
			// if err != nil {
			// 	log.Fatalf("Could not fetch registered commands: %v", err)
			// }

			for _, v := range registeredCommands {
				err := s.ApplicationCommandDelete(BotID, *GuildID, v.ID)
				if err != nil {
					log.Panicf("Cannot delete '%v' command: %v", v.Name, err)
				}
			}
		}

		log.Println("Gracefully shutting down.")
}
