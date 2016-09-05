package main

type StringSliceFlag []string

func (flag StringSliceFlag) String() string {
	returnString := "["
	for i, s := range flag {
		if i > 0 {
			returnString += " "
		}
		returnString += s
	}
	returnString += "]"
	return returnString
}

func (flag *StringSliceFlag) Set(value string) error {
	*flag = append(*flag, value)
	return nil
}
