package product

import (
	"errors"
	"sort"
)

type AssemblyAIAgentDefinition struct {
	VoiceKey string
	Name     string
	Payload  map[string]any
}

// AssemblyAIAgentProvisioningDefinitions returns the nine reusable agent
// configurations. Tools are client-handled, so AssemblyAI returns tool.call
// events to the backend instead of making outbound HTTP requests itself.
func AssemblyAIAgentProvisioningDefinitions() ([]AssemblyAIAgentDefinition, error) {
	tools, err := newAssistantToolRegistry().assemblyAIAgentTools()
	if err != nil {
		return nil, errors.New("assistant tool configuration is invalid")
	}
	keys := make([]string, 0, len(assemblyAILiveVoiceChoices))
	for key := range assemblyAILiveVoiceChoices {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	definitions := make([]AssemblyAIAgentDefinition, 0, len(keys))
	for _, key := range keys {
		voice := assemblyAILiveVoiceChoices[key]
		name := "Askolo Development - " + voice.Name
		definitions = append(definitions, AssemblyAIAgentDefinition{
			VoiceKey: key,
			Name:     name,
			Payload: map[string]any{
				"name":          name,
				"system_prompt": voice.SystemPrompt,
				"greeting":      voice.Greeting,
				"voice":         map[string]string{"voice_id": voice.VoiceID},
				"input": map[string]any{
					"format": map[string]any{"encoding": "audio/pcm", "sample_rate": 24000},
				},
				"output": map[string]any{
					"voice":  voice.VoiceID,
					"format": map[string]any{"encoding": "audio/pcm", "sample_rate": 24000},
				},
				"tools": tools,
			},
		})
	}
	return definitions, nil
}
