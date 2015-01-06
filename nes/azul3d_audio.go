// +build !sdl

package nes

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"azul3d.org/native/al.v1-dev"
)

type Azul3DAudio struct {
	paused     bool
	frequency  int
	device     *al.Device
	source     uint32
	buffers    []uint32
	sampleSize int
	input      chan int16
}

func NewAudio(frequency int, sampleSize int) (audio *Azul3DAudio, err error) {
	var device *al.Device

	device, err = al.OpenDevice("", nil)

	if err != nil {
		return
	}

	sampleSize = frequency/4
	audio = &Azul3DAudio{
		frequency:  frequency,
		device:     device,
		sampleSize: sampleSize,
		buffers:    make([]uint32, frequency/sampleSize),
		input:      make(chan int16, sampleSize),
	}
	fmt.Println(len(audio.buffers), "buffers")

	al.SetErrorHandler(func(e error) {
		err = e
	})

	device.GenSources(1, &audio.source)

	if !device.IsSource(audio.source) {
		err = errors.New("IsSource returned false")
		return
	}

	device.Sourcei(audio.source, al.LOOPING, al.FALSE)

	device.GenBuffers(int32(len(audio.buffers)), &audio.buffers[0])

	for i := range audio.buffers {
		if !device.IsBuffer(audio.buffers[i]) {
			err = errors.New(fmt.Sprintf("IsBuffer[%v] returned false", i))
			return
		}
	}

	return
}

func (audio *Azul3DAudio) Input() chan int16 {
	return audio.input
}

func (audio *Azul3DAudio) stream(schan chan []int16) {
	samples := []int16{}

	for {
		samples = append(samples, <-audio.input)

		if len(samples) == audio.sampleSize {
			schan <- samples
			samples = []int16{}
		}
	}
}

func (audio *Azul3DAudio) bufferData(buffer uint32, samples []int16) (err error) {
	al.SetErrorHandler(func(e error) {
		err = e
	})

	audio.device.BufferData(buffer, al.FORMAT_MONO16, unsafe.Pointer(&samples[0]),
		int32(int(unsafe.Sizeof(samples[0]))*len(samples)), int32(audio.frequency))

	return
}

func (audio *Azul3DAudio) Run() {
	running := true

	handler := func(e error) {
		if e != nil {
			fmt.Fprintf(os.Stderr, "%v\n", e)
			running = false
		}
	}

	al.SetErrorHandler(handler)

	schan := make(chan []int16, 256)
	empty := make([]int16, audio.sampleSize)

	go audio.stream(schan)

	for i := range audio.buffers {
		handler(audio.bufferData(audio.buffers[i], empty))
	}

	audio.device.SourceQueueBuffers(audio.source, audio.buffers)
	audio.device.SourcePlay(audio.source)

	state := al.PLAYING
	processed := int32(0)

	for running {
		if audio.device.GetSourcei(audio.source, al.BUFFERS_PROCESSED, &processed); processed > 0 {
			pbuffers := make([]uint32, processed)

			audio.device.SourceUnqueueBuffers(audio.source, pbuffers)

			fmt.Println("Processed:", processed)
			for i := range pbuffers {
				handler(audio.bufferData(pbuffers[i], <-schan))
			}
			fmt.Println("Refilled:", processed)

			audio.device.SourceQueueBuffers(audio.source, pbuffers)

			if audio.device.GetSourcei(audio.source, al.SOURCE_STATE, &state); state != al.PLAYING {
				audio.device.SourcePlay(audio.source)
			}
		}
	}
}

func (audio *Azul3DAudio) TogglePaused() {
	audio.device.SourcePause(audio.source)
	audio.paused = !audio.paused
}

func (audio *Azul3DAudio) Close() {
	audio.device.DeleteSources(1, &audio.source)
	audio.device.DeleteBuffers(int32(len(audio.buffers)), &audio.buffers[0])
	audio.device.Close()
}
