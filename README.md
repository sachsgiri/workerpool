# workerpool

##How to get
You can get this package using
$ go get github.com/sachsgiri/workerpool

##How to use 
//In your main.go

type Work struct {
	Name      string "The Name of a person"
	BirthYear int    "The Year the person was born"
	WP        *workerpool.Pool
}
func (work *Work) DoWork(workRoutine int) {
	time.Sleep(1 * time.Second)
	fmt.Printf("finished work for %s\n", work.Name)
	//panic("test")
}
func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	var number int32
	//number of routines, que capacity
	pool := workerpool.New(runtime.NumCPU(), 10)

		for i := 0; i < 10; i++ {
			go func() {
				work := &Work{
					Name: "Person-" + strconv.Itoa(int(atomic.AddInt32(&number, 1))),
					BirthYear: i,
					WP: pool,
				}
				fmt.Printf("Posting work for %s\n", work.Name)

				result, err := pool.PostWork("name_routine", work)

				fmt.Printf("Status : %s\n", result)

				if err != nil {
					fmt.Printf("ERROR: %s\n", err)
				}
				time.Sleep(1* time.Second)
			}()
		}
}

