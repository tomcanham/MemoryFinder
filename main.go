package main

func main() {
	processName := "Spotify Premium"

	windowInfo := getWindowInfoByName(processName, false)
	findUint32(windowInfo.processID, 689878)
}
