package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"gopkg.in/hraban/opus.v2"
)

type PCMData struct {
	SampleRate int
	Channels   int
	Samples    []int16
}

func decodePCM(raw []byte, sampleRate int, channels int) (*PCMData, error) {
	if len(raw)%2 != 0 {
		return nil, fmt.Errorf("PCM inválido: tamanho ímpar")
	}

	samples := make([]int16, len(raw)/2)
	if err := binary.Read(bytes.NewReader(raw), binary.LittleEndian, samples); err != nil {
		return nil, err
	}

	return &PCMData{
		SampleRate: sampleRate,
		Channels:   channels,
		Samples:    samples,
	}, nil
}

// Upsampling com interpolação linear melhorada
func upsample2x(input []int16) []int16 {
	if len(input) == 0 {
		return []int16{}
	}

	out := make([]int16, len(input)*2)

	for i := 0; i < len(input); i++ {
		out[i*2] = input[i]

		if i < len(input)-1 {
			// Interpolação linear entre amostras
			out[i*2+1] = int16((int32(input[i]) + int32(input[i+1])) / 2)
		} else {
			// Última amostra: duplicar
			out[i*2+1] = input[i]
		}
	}
	return out
}

func monoToStereo(input []int16) []int16 {
	out := make([]int16, len(input)*2)
	for i, v := range input {
		out[i*2] = v   // Canal esquerdo
		out[i*2+1] = v // Canal direito
	}
	return out
}

func sendPCMToWebRTCTrack(track *webrtc.TrackLocalStaticSample, wavBytes []byte) error {
	pcm, err := decodePCM(wavBytes, 24000, 1)
	if err != nil {
		return err
	}

	if pcm.SampleRate != 24000 {
		return fmt.Errorf("esperado PCM 24kHz da OpenAI")
	}
	if pcm.Channels != 1 {
		return fmt.Errorf("esperado PCM mono da OpenAI")
	}

	pcm48 := upsample2x(pcm.Samples)
	stereo := monoToStereo(pcm48)

	enc, err := opus.NewEncoder(48000, 2, opus.AppAudio)
	if err != nil {
		return err
	}

	enc.SetBitrate(64000) // 64kbps é bom para voz
	enc.SetComplexity(10) // Máxima qualidade

	const frameSize = 960              // 20ms em 48kHz
	const frameSamples = frameSize * 2 // stereo (L+R)
	const frameDuration = 20 * time.Millisecond

	timestamp := time.Duration(0)

	for i := 0; i+frameSamples <= len(stereo); i += frameSamples {
		frame := stereo[i : i+frameSamples]

		buf := make([]byte, 4000)

		n, err := enc.Encode(frame, buf)
		if err != nil {
			return fmt.Errorf("erro ao encodar frame: %w", err)
		}

		// Criar cópia dos dados encodados
		data := make([]byte, n)
		copy(data, buf[:n])

		sample := media.Sample{
			Data:     data,
			Duration: frameDuration,
		}

		if writeErr := track.WriteSample(sample); writeErr != nil {
			return fmt.Errorf("erro ao escrever sample: %w", writeErr)
		}

		timestamp += frameDuration
		time.Sleep(frameDuration)
	}

	// Processar samples residuais (se houver)
	remainder := len(stereo) % frameSamples
	if remainder > 0 {
		// Preencher último frame com zeros (silêncio)
		lastFrame := make([]int16, frameSamples)
		copy(lastFrame, stereo[len(stereo)-remainder:])

		buf := make([]byte, 4000)
		n, err := enc.Encode(lastFrame, buf)
		if err != nil {
			return fmt.Errorf("erro ao encodar frame residual: %w", err)
		}

		data := make([]byte, n)
		copy(data, buf[:n])

		sample := media.Sample{
			Data:     data,
			Duration: frameDuration,
		}

		if writeErr := track.WriteSample(sample); writeErr != nil {
			return fmt.Errorf("erro ao escrever sample residual: %w", writeErr)
		}
	}

	return nil
}
