
## 任务

1. 打印所有行数
2. 第8列包含名字， 打印 432 和 43243 个名字
3. 第5列包含日期，计算每月一共有多少捐款
4. 还是第8列，抽取出名字，计算出最常见的名字（first name），以及一共出现的次数

## Revision 0  基础版本

```
firstNamePat := regexp.MustCompile(", \\s*([^, ]+)")
names := make([]string, 0)
firstNames := make([]string, 0)
dates := make([]string, 0)
commonName := ""
commonCount := 0
```

scan 文件

```go
31	for scanner.Scan() {
32		text := scanner.Text()
33
34		// get all the names
35		split := strings.SplitN(text, "|", 9) // 10.95
36		name := strings.TrimSpace(split[7])
37		names = append(names, name)
38
39		// extract first names
40		if matches := firstNamePat.FindAllStringSubmatch(name, 1); len(matches) > 0 {
41			firstNames = append(firstNames, matches[0][1])
42		}
43
44		// extract dates
45		chars := strings.TrimSpace(split[4])[:6]
46		date := chars[:4] + "-" + chars[4:6]
47		dates = append(dates, date)
48	}
```

求解任务

```go
50	// report c2: names at index
51	fmt.Printf("Name: %s at index: %v\n", names[0], 0)
52	fmt.Printf("Name: %s at index: %v\n", names[432], 432)
53	fmt.Printf("Name: %s at index: %v\n", names[43243], 43243)
54	fmt.Printf("Name time: %v\n", time.Since(start))
55
56	// report c1: total number of lines
57	fmt.Printf("Total file line count: %v\n", len(names))
58	fmt.Printf("Line count time: : %v\n", time.Since(start))
59
60	// report c3: donation frequency
61	dateMap := make(map[string]int)
62	for _, date := range dates {
63		dateMap[date] += 1
64	}
65	for k, v := range dateMap {
66		fmt.Printf("Donations per month and year: %v and donation count: %v\n", k, v)
67	}
68	fmt.Printf("Donations time: : %v\n", time.Since(start))
69
70	// report c4: most common firstName
71	nameMap := make(map[string]int)
72	ncount := 0 // new count
73	for _, name := range firstNames {
74		ncount = nameMap[name] + 1
75		nameMap[name] = ncount
76		if ncount > commonCount {
77			commonName = name
78			commonCount = ncount
79		}
80	}
81
82	fmt.Printf("The most common first name is: %s and it occurs: %v times.\n", commonName, commonCount)
83	fmt.Printf("Most common name time: %v\n", time.Since(start))
```



这个版本 33秒

## Revision 1  无脑goroutine 版本😢

简单的想法：

- 从三个通道开始一个 goroutine 读取`nameC, lastnameC, datesC`以追加列表
- 对于每一行，为每一行启动一个 goroutine 并解析 3 个字段以将它们发送到这三个通道之一
- 等待所有 goroutine 完成
- 根据挑战规则报告（c1-c4）



每一行生成一个 goroutine，解析塞到结果channel里面去

```go
for scanner.Scan() {
		text := scanner.Text()
		wg.Add(3)
		go func() {
			// get all the names
			split := strings.SplitN(text, "|", 9)
			name := strings.TrimSpace(split[7])
			namesC <- name

			// extract first names
			if matches := firstNamePat.FindAllStringSubmatch(name, 1); len(matches) > 0 {
				firstNamesC <- matches[0][1]
			} else {
				wg.Add(-1)
			}

			// extract dates
			chars := strings.TrimSpace(split[4])[:6]
			date := chars[:4] + "-" + chars[4:6]
			datesC <- date
		}()
	}
```

然后有一个中的，select读取汇总，塞到结果list里面去

```go
go func() {
		for {
			select {
			case n, ok := <-namesC:
				if ok {
					names = append(names, n)
					wg.Done()
				}
			case fn, ok := <-firstNamesC:
				if ok {
					firstNames = append(firstNames, fn)
					wg.Done()
				}
			case d, ok := <-datesC:
				if ok {
					dates = append(dates, d)
					wg.Done()
				}
			}
		}
	}()
```





```
CPU-0: [read line1][read line2][read line3]
CPU-1:             [process line1]         [process line3]
CPU-2:                         [process line2]
```



```
send name      -> [ nameC      ] --\ 
send firstName -> [ firstNameC ] --- -> select() one -> [ append one to a list ]
send date      -> [ datesC     ] --/ 
```


12分钟  732秒

1. 性能差的原因，每行一个 goroutine，这个overhead太大了。而且消息传递太多了，开了3个chan，3x18 million = 54 million messages在传递 。

2. 第二个问题。通道，至少是容量为 1 的非缓冲通道，在向它们发送消息时会阻塞。Channel[被阻塞，](https://golang.org/ref/spec#Channel_types)直到它的接收端读取消息。sending goroutines 需要等待，因为 *for - select*  处理的goroutine有竞争。

这边作者使用了提个 1M行的小样本进行测试，下面有一堆分析goroutine的，待进一步学习。

我们在这些 Tracer 日志中看到的是两个 goroutine，一个读取行并启动 goroutine，一个接收消息，然后是一百万行解析 goroutine，每当它们向通道发送消息时，这些 goroutine 就会被分成三部分。当我们在跟踪开始时选择 ~120 毫秒之一时，我们看到的正是这一点。goroutine G1 发出系统调用以读取文件，然后启动行处理 goroutine。然后是一个名为 G6 的 goroutine，它代表 for-select 循环，从这些通道之一中排出消息并将数据附加到列表之一。

从跟踪日志的中间或末尾挑选一个跟踪器段，我们看到文件读取 goroutine 不再运行，G6 只是在那里挑选任何等待接收消息的 gorutines（G6 在 中等待[`runtime.selectgo()`](https://github.com/golang/go/blob/release-branch.go1.12/src/runtime/select.go#L105)）。所有 CPU 内核都饱和了，主要是等待从这些等待中的 goroutine 获取消息。然后总结为我们测量的 12 分钟。（有关 Tracer 的更详细说明，请参阅[附录 B4](https://marcellanz.com/post/file-read-challenge/#b4-gc-tracer-in-detail)）

并且通道必须等待其接收端读取消息，我们还应该发现大多数时候 goroutine 被拆分为三个部分。当我们将消息作为并发运行的 goroutine 发送到三个通道之一时，它们会分裂，因为 Go 正在调度 goroutine 在很可能一个 goroutine 将等待而另一个 goroutine 可能被调度运行的点。



##  Revision 2 – 减少Channels ，减少goroutine ⭐⭐⭐⭐⭐

减少channel，不要每种关心的数据就搞一个channel。 搞一个新的struct，把数据捏合在一起送到channel里。

```go
type entry struct {
		firstName string
		name      string
		date      string
		wg        *sync.WaitGroup
	}
```

每64k一个goroutine

```go
func main() {
    wg := sync.WaitGroup{}
    
    linesChunkLen := 64 * 1024
    lines := make([]string, 0, 0)
    
    for scanner.Scan() {
        line := scanner.Text()
        lines = append(lines, line)

        // lines 这里到了 64k
        //不过这边作者实现有个bug ，永远只处理了 64k * n 倍数的行数。剩下的没处理
        if len(lines) == linesChunkLen {

            //WaitGroup 对象内部有一个计数器，最初从0开始，它有三个方法：Add(),
            //Done(), Wait() 用来控制计数器的数量。Add(n) 把计数器设置为n ，
            //Done() 每次把计数器-1 ，wait() 会阻塞代码的运行，直到计数器地值减
            //为0。
            wg.Add(len(lines))

            process := lines
            go func() {
                for _, text := range process {
                    // process here !
                    
                    entries <- e
                }
            }()
            lines = make([]string, 0, linesChunkLen)
        }
    }
    wg.Wait()
}

```



花费 18.285秒

## Revision 3 – 减少channel通信⭐⭐⭐⭐

方案2 总共需要往chan里面发送18M数据 ，**不过处理18M的数据也太多了**，再次优化，每64k发送一块数据进行处理。

可以进一步减少chan 通信的开销，对于每 64k 行块，我们将相同数量的条目收集到一个切片中，并通过条目通道发送它们`entriesC`。

```go
	linesChunkLen := 64 * 1024
	lines := make([]string, 0, 0)
	scanner.Scan()
	for {
		lines = append(lines, scanner.Text())
		willScan := scanner.Scan()
		if len(lines) == linesChunkLen || !willScan {
			toProcess := lines
			wg.Add(len(toProcess))
			go func() {
				entries := make([]entry, 0, len(toProcess))
				for _, text := range toProcess {
					//process here
					entries = append(entries, entry)
				}
                
                //这样就是每64k一个entry
				entriesC <- entries
			}()
			lines = make([]string, 0, linesChunkLen)
		}
		if !willScan {
			break
		}
	}
```

通过这个计划，我们从版本 0 中减少了 60%时间。

我们又下降了 8 秒或 -42%

10.66832747s

## Revision 4 – Hang in on a Mutex （不一定有用）😢

我们仍然将解析行的块构建为条目，但我们不会通过通道共享它们以将它们附加到我们的三个列表中。相反，我们将附加到循环中的列表内联并使用[`sync.Mutex`](https://golang.org/pkg/sync/#Mutex). 这里我们删除了一个资源争用实例。我们没有通过通道发送条目块并在另一端由收集 goroutine 处理，而是通过调用等待，`mutex.Lock()`直到我们可以进入代码部分将解析的条目附加到三个列表中。

正如我们在这里看到的，我们不需要一个渠道来分享我们的数据来最终被收集。我们的 goroutine 可以使用它收集的数据在互斥锁上等待，直到互斥锁空闲。然后 goroutine 只是停在内存中，直到它被安排继续。Go 已为[`sync/mutex.go`](https://github.com/golang/go/blob/release-branch.go1.12/src/sync/mutex.go#L44). 我们的处理 goroutine 等待直到它可以将解析的数据附加到这些列表本身，而不是等待在通道上排出数据然后附加到列表中。



CPU 内核仍然得到很好的使用，因为我们一次运行 6 个 goroutine。在此修订版中，显式通道的代码组合结构消失了。我们没有获得太多，但我们暂时保留此修订更改。



10.31209465s



## Revision 5 – Inside the Loop（有用，但结构上未必合适）⭐



在这里，在捐赠时间和常用名称时间之间，我们需要大约 2 秒的时间来*遍历*firstNames 切片并找到最常用的名称。让我们尝试将它们，捐赠频率表和公共名称计数带入我们的解析循环。因此，在我们将行解析为条目后，当附加到列表时，我们更新循环内的日期频率和常用名称计数`Lines 68-81`。这样，我们不必遍历这两个映射，这样我们应该会获得一些秒数。

这里发生了什么事？我怀疑我们将解析的数据附加到我们的列表的代码部分，并没有完全饱和运行此代码部分的时间，与其他地方发生的一切有关。我们没有在解析过程中浪费这个时间，然后连续花时间循环映射并计算频率表和最常见的名称，而是将这段代码放在循环中。

> 人们可能会开始考虑我们在修订版 5 中所做的与可能的未来需求的扩展程度。在未来的挑战中或随着需求的变化，将这段代码放入循环中可能会对它的性能产生负面影响。一个合理的点。但我们在这里也是*为了表现*，当谈到量化表现时，这些数字不会撒谎。就*代码质量*而言，我并不是说我在所有情况下都支持这一点，但我们在这里仍然有一些乐趣，对吧？




## Revision 6 – Reuse Allocated Memory（不一定有用）😢

在我们的代码中，我们分配行切片和条目切片，然后我们收集数据行和解析字段的条目。一遍又一遍地分配这些 64k 大小的切片可能会对我们的性能产生负面影响，对吧？有了 a`sync.Pool`我们可以重用分配的切片，并且不需要重新分配。除了切片本身的分配之外，每当我们将一个元素附加到一个容量不足的切片时，Go 会将这些切片[增长](https://go.googlesource.com/go/+/refs/tags/go1.12.1/src/runtime/slice.go#76)2，容量小于 1024 和 1.25 以上

到目前为止，我们分配了起始容量为 0`linesChunkLen`的 line -slices 和 entry-slices。将它们分配到我们在处理 64k 行块期间肯定会增长的大小可能是有益的



减少了内存，时间减少和优化5相比不大。




## Revision 7: Regexp Usage⭐

正则表达式优化

```
non-capturing groups
```

使用regexp.FindString() 替换 firstNamePat.FindAllStringSubmatch

8.155115627s

## Revision 8: No Regexp⭐

不要使用正则表达式，直接使用string 分割，取var[0]

6.644675587s

## Revision 9 – Reduce the Garbage⭐⭐⭐

减少垃圾回收

```
names := make([]string, 0)
firstNames := make([]string, 0)
dates := make([]string, 0)
```

这些变量都不要存。 存日期不要使用string，优化格式。

3.880034076s

## 实际测试

```
0  Most common name time: 34.047600994s
1  Most common name time: 2m12.035417704s
2  Most common name time: 24.20121778s   <-
3  Most common name time: 10.822801266s  <- 
4  Most common name time: 10.990392216s
5  Most common name time: 7.682424486s   
6  Most common name time: 7.601689327s
7  Most common name time: 8.237632763s
8  Most common name time: 7.393345424s
9  Most common name time: 4.620049996s  <-

```




## 补充

> 1. 如何判断优化已经到达极限？ 

不parse，光读取一遍文件，也需要3.56s

> 2. 更进一步

使用 scanner.Bytes() 替换 scanner.Text() ，光读取一遍可以下降到2.392783786s

> 3. 核数的影响


```
GOMAXPROCS=1 go run ./rev9/readfile9.go itcont.txt
GOMAXPROCS=2 go run ./rev9/readfile9.go itcont.txt
GOMAXPROCS=4 go run ./rev9/readfile9.go itcont.txt
```

进行测试

