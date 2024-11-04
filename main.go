package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/mattermost/mattermost/server/public/model"
)

var Version = "development" // Default value - overwritten during bild process

var debugMode bool = false

// LogLevel is used to refer to the type of message that will be written using the logging code.
type LogLevel string

type mmConnection struct {
	mmURL    string
	mmPort   string
	mmScheme string
	mmToken  string
}

const (
	debugLevel   LogLevel = "DEBUG"
	infoLevel    LogLevel = "INFO"
	warningLevel LogLevel = "WARNING"
	errorLevel   LogLevel = "ERROR"
)

const (
	defaultPort   = "8065"
	defaultScheme = "http"
	pageSize      = 60
	maxErrors     = 3
)

type Team struct {
	Name         string
	ID           string
	ChannelCount int
}

type User struct {
	ID        string
	Username  string
	Email     string
	FirstName string
	LastName  string
	NickName  string
	Teams     []Team
}

// Logging functions

// LogMessage logs a formatted message to stdout or stderr
func LogMessage(level LogLevel, message string) {
	if level == errorLevel {
		log.SetOutput(os.Stderr)
	} else {
		log.SetOutput(os.Stdout)
	}
	log.SetFlags(log.Ldate | log.Ltime)
	log.Printf("[%s] %s\n", level, message)
}

// DebugPrint allows us to add debug messages into our code, which are only printed if we're running in debug more.
// Note that the command line parameter '-debug' can be used to enable this at runtime.
func DebugPrint(message string) {
	if debugMode {
		LogMessage(debugLevel, message)
	}
}

// getEnvWithDefaults allows us to retrieve Environment variables, and to return either the current value or a supplied default
func getEnvWithDefault(key string, defaultValue interface{}) interface{} {
	value, exists := os.LookupEnv(key)
	if !exists {
		return defaultValue
	}
	return value
}

func GetUserIDFromUsername(mmClient model.Client4, username string) (*User, error) {
	DebugPrint("Getting user ID for user: " + username)

	ctx := context.Background()
	etag := ""

	user, response, err := mmClient.GetUserByUsername(ctx, username, etag)

	if err != nil {
		LogMessage(errorLevel, "Failed to retrieve user: "+err.Error())
		return nil, err
	}
	if response.StatusCode != 200 {
		LogMessage(errorLevel, "Function call to GetUserByUsername returned bad HTTP response")
		return nil, errors.New("bad HTTP response")
	}

	mmUser := &User{
		ID:        user.Id,
		Username:  username,
		Email:     user.Email,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		NickName:  user.Nickname,
	}

	return mmUser, nil
}

func GetChannelCountForTeam(mmClient model.Client4, teamID string, userID string, countDMs bool) (int, int, error) {
	DebugPrint("Getting channel count for team ID: " + teamID)

	channelCount := 0
	dmChannelCount := 0
	ctx := context.Background()
	etag := ""

	channels, response, err := mmClient.GetChannelsForTeamForUser(ctx, teamID, userID, false, etag)

	if err != nil {
		LogMessage(errorLevel, "Failed to retrieve channels: "+err.Error())
		return -1, -1, err
	}
	if response.StatusCode != 200 {
		LogMessage(errorLevel, "Function call to GetChannelsForTeamForUser returned bad HTTP response")
		return -1, -1, errors.New("bad HTTP response")
	}

	for _, channel := range channels {
		if channel.Type == "D" {
			if countDMs {
				dmChannelCount++
			}
		} else {
			channelCount++
		}
	}

	return channelCount, dmChannelCount, nil
}

func GetTeamsForUser(mmClient model.Client4, userID string) ([]Team, error) {

	DebugPrint("Getting teams for user ID: " + userID)

	ctx := context.Background()
	etag := ""

	teams, response, err := mmClient.GetTeamsForUser(ctx, userID, etag)
	if err != nil {
		LogMessage(errorLevel, "Failed to retrieve teams: "+err.Error())
		return nil, err
	}
	if response.StatusCode != 200 {
		LogMessage(errorLevel, "Function call to GetTeamsForUser returned bad HTTP response")
		return nil, errors.New("bad HTTP response")
	}

	var teamsList []Team

	for _, mmTeam := range teams {
		team := Team{
			Name: mmTeam.DisplayName,
			ID:   mmTeam.Id,
		}

		teamsList = append(teamsList, team)
	}

	return teamsList, nil
}

func PrintSummary(user User, totalDMChannels int) {

	totalChannelCount := 0

	fmt.Printf("\n\n")
	fmt.Printf("Summary\n")
	fmt.Printf("=======\n\n")
	fmt.Printf("Username: %s\n", user.Username)
	fmt.Printf("Email:    %s\n", user.Email)
	fmt.Printf("Name:     %s %s\n", user.FirstName, user.LastName)
	fmt.Printf("Nickname: %s\n\n", user.NickName)
	fmt.Printf("Teams\n")
	fmt.Printf("=====\n\n")

	// Figure out the longest team name to assist with formatting
	maxTeamNameLength := 0
	for _, team := range user.Teams {
		if len(team.Name) > maxTeamNameLength {
			maxTeamNameLength = len(team.Name)
		}
	}

	// Add some padding
	maxTeamNameLength += 2

	// Now we can print the Teams portion
	for _, team := range user.Teams {
		fmt.Printf("%-*s : %d\n", maxTeamNameLength, team.Name, team.ChannelCount)
		totalChannelCount += team.ChannelCount
	}

	fmt.Printf("\nDirect Message Channels : %d\n", totalDMChannels)
	fmt.Printf("\nTotal channel count     : %d\n\n", totalChannelCount+totalDMChannels)
}

func main() {

	// Parse Command Line
	DebugPrint("Parsing command line")

	var MattermostURL string
	var MattermostPort string
	var MattermostScheme string
	var MattermostToken string
	var MattermostUser string
	var DebugFlag bool
	var VersionFlag bool

	flag.StringVar(&MattermostURL, "url", "", "The URL of the Mattermost instance (without the HTTP scheme)")
	flag.StringVar(&MattermostPort, "port", "", "The TCP port used by Mattermost. [Default: "+defaultPort+"]")
	flag.StringVar(&MattermostScheme, "scheme", "", "The HTTP scheme to be used (http/https). [Default: "+defaultScheme+"]")
	flag.StringVar(&MattermostToken, "token", "", "The auth token used to connect to Mattermost")
	flag.StringVar(&MattermostUser, "user", "", "The username of the Mattermost user")
	flag.BoolVar(&DebugFlag, "debug", false, "Enable debug output")
	flag.BoolVar(&VersionFlag, "version", false, "Show version information and exit")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options]\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "This utility is used to find how many channels a users is member of.")
		fmt.Fprintln(flag.CommandLine.Output(), "Options:")
		flag.PrintDefaults()
	}

	flag.Parse()

	if VersionFlag {
		fmt.Printf("mm-channel-count - Version: %s\n\n", Version)
		os.Exit(0)
	}

	// If information not supplied on the command line, check whether it's available as an envrionment variable
	if MattermostURL == "" {
		MattermostURL = getEnvWithDefault("MM_URL", "").(string)
	}
	if MattermostPort == "" {
		MattermostPort = getEnvWithDefault("MM_PORT", defaultPort).(string)
	}
	if MattermostScheme == "" {
		MattermostScheme = getEnvWithDefault("MM_SCHEME", defaultScheme).(string)
	}
	if MattermostToken == "" {
		MattermostToken = getEnvWithDefault("MM_TOKEN", "").(string)
	}
	if !DebugFlag {
		DebugFlag = getEnvWithDefault("MM_DEBUG", debugMode).(bool)
	}

	DebugMessage := fmt.Sprintf("Parameters: \n  MattermostURL=%s\n  MattermostPort=%s\n  MattermostScheme=%s\n  MattermostToken=%s\n  User=%s\n",
		MattermostURL,
		MattermostPort,
		MattermostScheme,
		MattermostToken,
		MattermostUser)
	DebugPrint(DebugMessage)

	// Validate required parameters
	DebugPrint("Validating parameters")
	var cliErrors bool = false
	if MattermostURL == "" {
		LogMessage(errorLevel, "The Mattermost URL must be supplied either on the command line of vie the MM_URL environment variable")
		cliErrors = true
	}
	if MattermostScheme == "" {
		LogMessage(errorLevel, "The Mattermost HTTP scheme must be supplied either on the command line of vie the MM_SCHEME environment variable")
		cliErrors = true
	}
	if MattermostToken == "" {
		LogMessage(errorLevel, "The Mattermost auth token must be supplied either on the command line of vie the MM_TOKEN environment variable")
		cliErrors = true
	}
	if MattermostUser == "" {
		LogMessage(errorLevel, "A Mattermost username is required to use this utility.")
		cliErrors = true
	}

	if cliErrors {
		flag.Usage()
		os.Exit(1)
	}

	debugMode = DebugFlag

	// Prepare the Mattermost connection
	mattermostConenction := mmConnection{
		mmURL:    MattermostURL,
		mmPort:   MattermostPort,
		mmScheme: MattermostScheme,
		mmToken:  MattermostToken,
	}

	mmTarget := fmt.Sprintf("%s://%s:%s", mattermostConenction.mmScheme, mattermostConenction.mmURL, mattermostConenction.mmPort)

	DebugPrint("Full target for Mattermost: " + mmTarget)
	mmClient := model.NewAPIv4Client(mmTarget)
	mmClient.SetToken(mattermostConenction.mmToken)
	DebugPrint("Connected to Mattermost")

	LogMessage(infoLevel, "Processing started - Version: "+Version)

	// Get the ID (and other information) of the user
	user, err := GetUserIDFromUsername(*mmClient, MattermostUser)
	if err != nil {
		LogMessage(errorLevel, "Failed to retrieve user from Mattermost")
		os.Exit(10)
	}

	// Get the teams that this user is a member of
	teams, err := GetTeamsForUser(*mmClient, user.ID)
	if err != nil {
		LogMessage(errorLevel, "Failed to retrieve teams from Mattermost")
		os.Exit(11)
	}

	user.Teams = teams
	var totalDMChannels int
	firstTeam := true

	for i := range teams {
		var teamChannelCount, dmChannelCount int

		// We only need to count the DMs for the first team, as they'll be common across all teams
		// for a given user and Mattermost connection.
		if firstTeam {
			teamChannelCount, dmChannelCount, err = GetChannelCountForTeam(*mmClient, teams[i].ID, user.ID, true)
			totalDMChannels = dmChannelCount
			firstTeam = false
		} else {
			teamChannelCount, _, err = GetChannelCountForTeam(*mmClient, teams[i].ID, user.ID, false)
		}
		if err != nil {
			LogMessage(warningLevel, "Failed to get channel count for team "+teams[i].Name)
		}
		teams[i].ChannelCount = teamChannelCount
	}

	PrintSummary(*user, totalDMChannels)
}
