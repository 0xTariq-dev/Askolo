package product

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const assemblyAILiveTestOptInEnv = "ASKOLO_RUN_LIVE_ASSEMBLYAI_TESTS"

const assemblyAILiveEnglishFixture = "assemblyai-live-english-16k.wav"

func TestAssemblyAILiveEnglishFixtureFormat(t *testing.T) {
	wav, err := os.ReadFile(filepath.Join("testdata", assemblyAILiveEnglishFixture))
	if err != nil {
		t.Fatalf("read synthetic English audio fixture: %v", err)
	}
	pcm, err := parseAssemblyAIRealtimePCM16WAV(wav)
	if err != nil {
		t.Fatalf("parse synthetic English audio fixture: %v", err)
	}
	if len(pcm) < assemblyAIRealtimeMinPCMFrameBytes ||
		len(pcm) > assemblyAIRealtimeSampleRateHz*2*10 {
		t.Fatalf("synthetic English fixture audio length is outside the expected short-test range")
	}
}

// This opt-in test calls AssemblyAI directly, so it does not require an Askolo
// account balance. It consumes AssemblyAI provider usage while its WebSocket
// session is open. Keep the regular mocked tests as the default CI coverage.
//
// The synthetic fixture says: "Hello there. This is a short English transcription test."
// Run explicitly from the repository root with:
//
//	cd services/askolo-backend && ASKOLO_RUN_LIVE_ASSEMBLYAI_TESTS=1 go test \
//	  ./internal/modules/product -run '^TestAssemblyAILiveEnglishFixture$' -count=1 -v
func TestAssemblyAILiveEnglishFixture(t *testing.T) {
	if os.Getenv(assemblyAILiveTestOptInEnv) != "1" {
		t.Skipf("set %s=1 to run the billable live AssemblyAI test", assemblyAILiveTestOptInEnv)
	}

	apiKey := strings.TrimSpace(os.Getenv("ASSEMBLY_AI_API_KEY"))
	if apiKey == "" {
		t.Fatal("ASSEMBLY_AI_API_KEY must be configured to run the live AssemblyAI test")
	}

	wav, err := os.ReadFile(filepath.Join("testdata", assemblyAILiveEnglishFixture))
	if err != nil {
		t.Fatalf("read synthetic English audio fixture: %v", err)
	}
	pcm, err := parseAssemblyAIRealtimePCM16WAV(wav)
	if err != nil {
		t.Fatalf("parse synthetic English audio fixture: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	provider := newAssemblyAIClient(
		apiKey,
		"",
		"",
		"",
		&http.Client{Timeout: 10 * time.Second},
		nil,
	)
	token, err := requestAssemblyAIRealtimeTokenWithSessionDuration(
		ctx,
		&http.Client{Timeout: 10 * time.Second},
		assemblyAIRealtimeTokenBaseURL,
		apiKey,
		assemblyAIRealtimeMinProviderSessionSeconds,
	)
	if err != nil {
		t.Fatal("could not create a short-lived AssemblyAI streaming token")
	}
	session, err := provider.OpenRealtimeSession(ctx, token)
	if err != nil {
		t.Fatal("could not open an AssemblyAI real-time transcription session")
	}

	terminationSent := false
	defer func() {
		if !terminationSent {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = session.SendTermination(cleanupCtx)
			cleanupCancel()
		}
		session.Close()
	}()

	beginCtx, beginCancel := context.WithTimeout(ctx, 8*time.Second)
	for {
		event, readErr := session.ReadProviderMessage(beginCtx)
		if readErr != nil {
			beginCancel()
			t.Fatal("AssemblyAI did not begin the streaming session")
		}
		if event.Type == "Begin" {
			break
		}
	}
	beginCancel()

	if err := streamAssemblyAIRealtimeFixture(ctx, session, pcm); err != nil {
		t.Fatal("could not stream the synthetic English fixture to AssemblyAI")
	}
	if err := session.SendTermination(ctx); err != nil {
		t.Fatal("could not terminate the AssemblyAI streaming session")
	}
	terminationSent = true

	var finalTranscript string
	terminationCtx, terminationCancel := context.WithTimeout(ctx, 12*time.Second)
	defer terminationCancel()
	for {
		event, readErr := session.ReadProviderMessage(terminationCtx)
		if readErr != nil {
			t.Fatal("AssemblyAI did not finish the streaming session cleanly")
		}
		switch event.Type {
		case "Turn":
			var turn struct {
				Transcript string `json:"transcript"`
				EndOfTurn  bool   `json:"end_of_turn"`
			}
			if err := json.Unmarshal(event.Payload, &turn); err != nil {
				t.Fatal("AssemblyAI returned an invalid turn event")
			}
			if turn.EndOfTurn {
				finalTranscript = turn.Transcript
			}
		case "Termination":
			if !strings.Contains(strings.ToLower(finalTranscript), "hello") ||
				!strings.Contains(strings.ToLower(finalTranscript), "test") {
				t.Fatal("AssemblyAI's final transcript did not contain the fixture's expected English keywords")
			}
			return
		}
	}
}

func streamAssemblyAIRealtimeFixture(
	ctx context.Context,
	session assemblyAIRealtimeSession,
	pcm []byte,
) error {
	const chunkDuration = 100 * time.Millisecond
	chunkBytes := assemblyAIRealtimeSampleRateHz * 2 * int(chunkDuration/time.Millisecond) / 1000
	ticker := time.NewTicker(chunkDuration)
	defer ticker.Stop()

	for offset := 0; offset < len(pcm); {
		end := offset + chunkBytes
		if end > len(pcm) {
			end = len(pcm)
		}
		frame := pcm[offset:end]
		if len(frame) < assemblyAIRealtimeMinPCMFrameBytes {
			padded := make([]byte, assemblyAIRealtimeMinPCMFrameBytes)
			copy(padded, frame)
			frame = padded
		}
		if err := session.SendPCMFrame(ctx, frame); err != nil {
			return err
		}
		offset = end
		if offset < len(pcm) {
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return nil
}

func parseAssemblyAIRealtimePCM16WAV(wav []byte) ([]byte, error) {
	if len(wav) < 12 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return nil, errors.New("invalid WAV container")
	}

	var (
		formatSeen    bool
		dataSeen      bool
		pcmFormat     uint16
		channels      uint16
		sampleRate    uint32
		bitsPerSample uint16
		pcm           []byte
	)
	for offset := 12; offset+8 <= len(wav); {
		chunkSize := binary.LittleEndian.Uint32(wav[offset+4 : offset+8])
		chunkStart := offset + 8
		if uint64(chunkSize) > uint64(len(wav)-chunkStart) {
			return nil, errors.New("truncated WAV chunk")
		}
		chunkEnd := chunkStart + int(chunkSize)

		switch string(wav[offset : offset+4]) {
		case "fmt ":
			if chunkSize < 16 {
				return nil, errors.New("invalid WAV format chunk")
			}
			formatSeen = true
			pcmFormat = binary.LittleEndian.Uint16(wav[chunkStart : chunkStart+2])
			channels = binary.LittleEndian.Uint16(wav[chunkStart+2 : chunkStart+4])
			sampleRate = binary.LittleEndian.Uint32(wav[chunkStart+4 : chunkStart+8])
			bitsPerSample = binary.LittleEndian.Uint16(wav[chunkStart+14 : chunkStart+16])
		case "data":
			dataSeen = true
			pcm = wav[chunkStart:chunkEnd]
		}

		offset = chunkEnd
		if chunkSize%2 != 0 {
			offset++
		}
	}

	if !formatSeen || !dataSeen || pcmFormat != 1 || channels != 1 ||
		sampleRate != assemblyAIRealtimeSampleRateHz || bitsPerSample != 16 ||
		len(pcm) == 0 || len(pcm)%2 != 0 {
		return nil, errors.New("WAV must contain mono 16-bit PCM at 16 kHz")
	}
	return append([]byte(nil), pcm...), nil
}
