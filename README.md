Just playing around teaching myself Golang.

This project searches another process' memory space for a specified UInt32.

_TODO_

* Make it more efficient in how it loads memory -- right now it reads the ENTIRE block of memory specified by a MBI structure at once
* Enable searching for more than just UInt32
* Track hits
* Allow things like "see live values" and "watch changes"