package lfu

import (
	"fmt"
	"testing"
	"time"
)

var expTimeD = time.Second * 1

func TestLFU(t *testing.T) {
	c := NewLFUCache(1000, time.Second*30)
	expTime := time.Now().Add(expTimeD)
	c.Set("a", "a", &expTime)
	if v := c.Get("a"); v != "a" {
		t.Errorf("Value was not saved: %v != 'a'", v)
	}
	if l := c.Len(); l != 1 {
		t.Errorf("Length was not updated: %v != 1", l)
	}

	c.Set("b", "b", &expTime)
	if v := c.Get("b"); v != "b" {
		t.Errorf("Value was not saved: %v != 'b'", v)
	}
	if l := c.Len(); l != 2 {
		t.Errorf("Length was not updated: %v != 2", l)
	}

	c.Get("a")
	evicted := c.Evict(1)
	if v := c.Get("a"); v != "a" {
		t.Errorf("Value was improperly evicted: %v != 'a'", v)
	}
	if v := c.Get("b"); v != nil {
		t.Errorf("Value was not evicted: %v", v)
	}
	if l := c.Len(); l != 1 {
		t.Errorf("Length was not updated: %v != 1", l)
	}
	if evicted != 1 {
		t.Errorf("Number of evicted items is wrong: %v != 1", evicted)
	}
}

func TestBoundsMgmt(t *testing.T) {
	c := NewLFUCache(5, time.Second*20)
	expTime := time.Now().Add(expTimeD)

	for i := 0; i < 100; i++ {
		c.Set(fmt.Sprintf("%v", i), i, &expTime)
	}
	if c.Len() > 10 {
		t.Errorf("Bounds management failed to evict properly: %v", c.Len())
	}
}

func TestEviction(t *testing.T) {
	ch := make(chan Eviction, 1)
	expTime := time.Now().Add(expTimeD)
	c := NewLFUCache(1000, time.Second*20)
	c.EvictionChannel = ch
	c.Set("a", "b", &expTime)
	c.Evict(1)

	ev := <-ch

	if ev.Key != "a" || ev.Value.(string) != "b" {
		t.Error("Incorrect item")
	}
}

func TestEexp(t *testing.T) {
	expTime := time.Now().Add(time.Millisecond * 10)
	c := NewLFUCache(1000, time.Second*30)
	c.Set("a", "b", &expTime)
	res := c.Get("a")
	if res == nil || res != "b" {
		t.Error("set failed")
	}
	time.Sleep(time.Millisecond * 20)
	res = c.Get("a")
	if res != nil {
		t.Error("expire failed")
	}
}

func TestCleaner(t *testing.T) {
	expTime := time.Now().Add(time.Millisecond * 10)
	c := NewLFUCache(1000, time.Millisecond*30)
	c.Set("a", "b", &expTime)
	res := c.Get("a")
	if res == nil || res != "b" || c.len != 1 `{
		t.Error("set failed")
	}
	time.Sleep(time.Millisecond * 40)
	if c.len != 0 {
		t.Error("cleaner expire failed, ", res)
	}
}
