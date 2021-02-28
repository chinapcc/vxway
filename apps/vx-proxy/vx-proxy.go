package main

import "flag"

// begin FilterFlag
type filterFlag []string
func (f *filterFlag) String() string  {
	return "filter flag"
}
func (f *filterFlag) Set(value string) error  {
	*f = append(*f,value)
	return nil
}
// end FilterFlag

var (
	defaultFilters = &filterFlag{}
	filters = &filterFlag{}

	addr = flag.String("addr","127.0.0.1:80","Addr: http rrequest entrypoint")
)


func init() {
	defaultFilters.Set()
}

/**
* 主入口程序
 */
func main() {

}