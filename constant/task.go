package constant

import "strconv"

type TaskPlatform string

const (
	TaskPlatformSuno             TaskPlatform = "suno"
	TaskPlatformMidjourney                    = "mj"
	TaskPlatformOpenAIVideo                   = "openai_video"
	TaskPlatformSeedance                      = "seedance_async"
	TaskPlatformSeedanceDiscount              = "seedance_discount"
)

// IsVideoTaskPlatform reports whether platform is a known persisted video-task
// platform. Unknown and non-video platforms must remain refundable.
func IsVideoTaskPlatform(platform TaskPlatform) bool {
	if platform == TaskPlatformOpenAIVideo || platform == TaskPlatformSeedance || platform == TaskPlatformSeedanceDiscount {
		return true
	}

	channelType, err := strconv.Atoi(string(platform))
	if err != nil {
		return false
	}

	switch channelType {
	case ChannelTypeAli,
		ChannelTypeKling,
		ChannelTypeJimeng,
		ChannelTypeVertexAi,
		ChannelTypeVidu,
		ChannelTypeDoubaoVideo,
		ChannelTypeVolcEngine,
		ChannelTypeSora,
		ChannelTypeOpenAI,
		ChannelTypeGemini,
		ChannelTypeMiniMax:
		return true
	default:
		return false
	}
}

const (
	SunoActionMusic  = "MUSIC"
	SunoActionLyrics = "LYRICS"

	TaskActionGenerate          = "generate"
	TaskActionTextGenerate      = "textGenerate"
	TaskActionFirstTailGenerate = "firstTailGenerate"
	TaskActionReferenceGenerate = "referenceGenerate"
	TaskActionRemix             = "remixGenerate"
)

var SunoModel2Action = map[string]string{
	"suno_music":  SunoActionMusic,
	"suno_lyrics": SunoActionLyrics,
}
