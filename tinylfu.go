// Package tinylfu is an implementation of the TinyLFU caching algorithm
/*
   http://arxiv.org/abs/1512.00727
*/
package tinylfu

import (
	"container/list"
	"fmt"

	"github.com/dgryski/go-metro"
)

type T struct {
	c       *cm4
	bouncer *doorkeeper
	w       int
	samples int
	lru     *lruCache
	slru    *slruCache
	data    map[string]*list.Element
	onEvict func(interface{})
}

type Config struct {
	Size    int
	Samples int
	OnEvict func(interface{})
}

func New(cfg Config) *T {

	const lruPct = 1

	lruSize := (lruPct * cfg.Size) / 100
	if lruSize < 1 {
		lruSize = 1
	}
	slruSize := int(float64(cfg.Size) * ((100.0 - lruPct) / 100.0))
	if slruSize < 1 {
		slruSize = 1

	}
	slru20 := int(0.2 * float64(slruSize))
	if slru20 < 1 {
		slru20 = 1
	}

	data := make(map[string]*list.Element, cfg.Size)
	onEvict := cfg.OnEvict
	if onEvict == nil {
		onEvict = func(interface{}) {}
	}

	return &T{
		c:       newCM4(cfg.Size),
		w:       0,
		samples: cfg.Samples,
		bouncer: newDoorkeeper(cfg.Samples, 0.01),

		data: data,

		lru:  newLRU(lruSize, data),
		slru: newSLRU(slru20, slruSize-slru20, data),

		onEvict: onEvict,
	}
}

func (t *T) Get(key string) (interface{}, bool) {

	t.w++
	if t.w == t.samples {
		t.c.reset()
		t.bouncer.reset()
		t.w = 0
	}

	val, ok := t.data[key]
	if !ok {
		keyh := metro.Hash64Str(key, 0)
		t.c.add(keyh)
		return nil, false
	}

	item := val.Value.(*slruItem)

	t.c.add(item.keyh)

	v := item.value
	if item.listid == 0 {
		t.lru.get(val)
	} else {
		t.slru.get(val)
	}

	return v, true
}

func (t *T) Add(key string, val interface{}) {

	newitem := slruItem{0, key, val, metro.Hash64Str(key, 0)}

	oitem, evicted := t.lru.add(newitem)
	if !evicted {
		return
	}

	fmt.Println("Add / old item -> ", oitem.value)

	// estimate count of what will be evicted from slru
	victim := t.slru.victim()
	if victim == nil {
		fmt.Println("Add victim == nil / old item -> ", oitem.value)
		t.slru.add(oitem)
		return
	}

	if !t.bouncer.allow(oitem.keyh) {
		fmt.Println("Add bouncer !allow / old item -> ", oitem.value)
		return
	}

	vcount := t.c.estimate(victim.keyh)
	ocount := t.c.estimate(oitem.keyh)

	if ocount < vcount {
		fmt.Println("Add ocount < vcount / old item -> ", oitem.value)
		return
	}

	fmt.Println("Add final slru.add / old item -> ", oitem.value)
	t.slru.add(oitem)
}
