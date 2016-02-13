package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/boomlinde/acidforth/collection"
	"github.com/boomlinde/acidforth/machine"
	"github.com/boomlinde/acidforth/midi"
	"github.com/boomlinde/acidforth/synth"
	"github.com/gordonklaus/portaudio"
	"github.com/rakyll/portmidi"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
)

const sfreq = 44100

func main() {
	var listMidi bool
	var midiInterface int
	var m *midi.Midi
	var prompt float64

	flag.BoolVar(&listMidi, "l", false, "List MIDI interfaces")
	flag.IntVar(&midiInterface, "m", -1, "Connect to MIDI interface ID")
	flag.Parse()
	args := flag.Args()

	if listMidi {
		portmidi.Initialize()
		defer portmidi.Terminate()
		deviceCount := portmidi.CountDevices()
		for i := 0; i < deviceCount; i++ {
			fmt.Println(i, portmidi.GetDeviceInfo(portmidi.DeviceId(i)))
		}
		os.Exit(0)
	} else if midiInterface != -1 {
		portmidi.Initialize()
		defer portmidi.Terminate()
		in, err := portmidi.NewInputStream(portmidi.DeviceId(midiInterface), 1024)
		if err != nil {
			log.Fatal(err)
		}
		defer in.Close()
		m = midi.NewMidi(in.Listen())
	} else {
		m = midi.NewMidi(make(chan portmidi.Event))
	}

	pl := &sync.Mutex{}

	col := collection.NewCollection()

	m.Register(col)
	go m.Listen()

	addComponents(sfreq, col, args[:len(args)-1])

	col.Machine.Register("prompt", func(s *machine.Stack) {
		pl.Lock()
		s.Push(prompt)
		pl.Unlock()
	})

	log.Println("Compiling program")
	err := col.Machine.Build(args[len(args)-1])
	if err != nil {
		log.Println("ERROR:", err)
	}
	log.Println("Running")

	portaudio.Initialize()
	defer portaudio.Terminate()
	stream, err := portaudio.OpenDefaultStream(0, 2, sfreq, 0, col.Callback)
	chk(err)
	defer stream.Close()
	stream.Start()

	reader := bufio.NewReader(os.Stdin)
	numberRe, err := regexp.Compile("[0-9]+\\.?[0-9]*")
	chk(err)

	for {
		text, err := reader.ReadString('\n')
		chk(err)
		found := numberRe.FindString(text)
		if found == "" {
			col.Playing = !col.Playing
			if col.Playing == true {
				log.Println("Starting sequencer")
			} else {
				log.Print("Stopping sequencer")
			}
		} else {
			tpr, err := strconv.ParseFloat(found, 64)
			if err != nil {
				log.Println(err)
				continue
			}
			pl.Lock()
			prompt = tpr
			pl.Unlock()
		}
	}
}

func addComponents(srate float64, c *collection.Collection, samples []string) {
	for i := 1; i < 9; i++ {
		_ = synth.NewOperator(fmt.Sprintf("op%d", i), c, srate)
		_ = synth.NewEnvelope(fmt.Sprintf("env%d", i), c, srate)
	}
	for _, r := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		_ = synth.NewRegister(string(r), c)
	}
	for i := 1; i < 5; i++ {
		_ = synth.NewAccumulator(fmt.Sprintf("mix%d", i), c)
		_ = synth.NewDelay(fmt.Sprintf("delay%d", i), c, srate)
	}
	for i := 1; i < 9; i++ {
		_ = synth.NewDSeq(fmt.Sprintf("dseq%d", i), c)
	}
	for _, v := range samples {
		s, err := os.Stat(v)
		if err != nil {
			log.Fatal(err)
		}
		if s.Mode().IsDir() {
			files, err := filepath.Glob(filepath.Join(v, "*.wav"))
			if err != nil {
				log.Fatal(err)
			}
			for _, f := range files {
				_ = synth.NewSampler(f, c, srate)
			}
		} else {
			_ = synth.NewSampler(v, c, srate)
		}
	}

	_ = synth.NewSeq("seq", c, srate)

	synth.NewWaveTables(c)

	c.Machine.Register("srate", func(s *machine.Stack) { s.Push(srate) })
	c.Machine.Register("m2f", func(s *machine.Stack) {
		s.Push(440 * math.Pow(2, (s.Pop()-69)/12))
	})
}

func chk(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
