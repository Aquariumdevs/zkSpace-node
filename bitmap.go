package main

/*
	func (app *App) countBitmap(byteSlice []byte) int {
		//counts the number of true bits in the bitmap
		var counter int
		for i := range byteSlice {
		    	for j := 0; j < 8; j++ {
		        	if byteSlice[i]&(1<<uint(j)) != 0 {
					counter ++
				}
			}
		}
		return counter
	}

	func (app *App) parseBitmap(byteSlice []byte) ([]bool, int) {
		// convert the byte slice to a bool array
		boolArray := make([]bool, len(byteSlice)*8)
		var counter int

		for i := range byteSlice {
		    	for j := 0; j < 8; j++ {
		        	boolArray[i*8+j] = byteSlice[i]&(1<<uint(j)) != 0
				if boolArray[i*8+j] {
					counter ++
				}
		    	}
		}
		return boolArray, counter
	}
*/
