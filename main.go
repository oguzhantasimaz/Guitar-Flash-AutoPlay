package main

import (
	"fmt"
	"image"
	"time"

	"github.com/go-vgo/robotgo"
	"github.com/kbinani/screenshot"
)

var currentTime = time.Now().UnixNano() / int64(time.Millisecond)
var previousReceiveTimeA int64 = 0
var previousReceiveTimeS int64 = 0
var previousReceiveTimeJ int64 = 0
var previousReceiveTimeK int64 = 0
var previousReceiveTimeL int64 = 0

func main() {
	x, y, width, height := 0, 0, 1512, 982

	//wait 5 seconds before starting
	time.Sleep(5 * time.Second)

	for {
		img, err := screenshot.Capture(x, y, width, height)
		if err != nil {
			fmt.Println("Failed to capture screen:", err)
			return
		}

		go pressNote(534, 673, "a", &previousReceiveTimeA, img)
		go pressNote(667, 673, "s", &previousReceiveTimeS, img)
		go pressNote(760, 673, "j", &previousReceiveTimeJ, img)
		go pressNote(866, 673, "k", &previousReceiveTimeK, img)
		go pressNote(970, 673, "l", &previousReceiveTimeL, img)
	}
}

// r,g,b 20,20,20 - background
func pressNote(x, y int, key string, pt *int64, screenshot *image.RGBA) {

	currentTime = time.Now().UnixNano() / int64(time.Millisecond)

	lightningColor := screenshot.At(361, 420)
	r1, g1, b1, _ := lightningColor.RGBA()

	if r1 == 65535 && g1 == 65535 && b1 == 65535 {
		return
	}

	color := screenshot.At(x, y)
	r, g, b, _ := color.RGBA()

	//if the color not close to 20,20,20 rgb values then press the key

	if r > 17000 && g > 17000 && b > 17000 {

		if currentTime-*pt < 100 {
			return
		}

		fmt.Println("pressing key", key)
		fmt.Println("current time", currentTime)
		fmt.Println("previous time", *pt)
		fmt.Println("time difference", currentTime-*pt)
		fmt.Println("COLOR: ", r, g, b)

		*pt = currentTime
		robotgo.MilliSleep(100)
		robotgo.KeyTap(key)
	}

	return
}

//first note is at 478, 817
//second note is at 618, 817
//third note is at 750, 817
//fourth note is at 888, 817
//fifth note is at 1018, 817
