package pingtrace

import (
	"testing"
	"time"
)

func openPings() []Ping {
	t0 := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	return []Ping{
		{Domain: "fidonet", Zone: 2, Net: 280, Node: 5555, Address: "2:280/5555", Token: "a1b2c3d4", MSGID: "2:5001/100 68b8a1c2", SentTime: t0},
		{Domain: "fidonet", Zone: 1, Net: 1, Node: 19, Address: "1:1/19", Token: "e5f6a7b8", MSGID: "2:5001/100 68b8a1c3", SentTime: t0},
		{Domain: "fidonet", Zone: 2, Net: 221, Node: 1, Address: "2:221/1", Token: "c9d0e1f2", MSGID: "2:5001/100 68b8a1c4", SentTime: t0},
		{Domain: "fidonet", Zone: 2, Net: 221, Node: 1, Address: "2:221/1", Mode: ModeDirect, Token: "00112233", MSGID: "2:5001/100 68b8a1c5", SentTime: t0.Add(time.Minute)},
	}
}

func TestMatchPrefersReplyKludge(t *testing.T) {
	r := Reply{ReplyID: "2:5001/100 68B8A1C3", FromAddr: "2:280/5555", Subject: "Pong: PING 2:280/5555 a1b2c3d4"}
	p := Match(r, openPings())
	if p == nil || p.Address != "1:1/19" {
		t.Fatalf("REPLY kludge must win over the subject token: %+v", p)
	}
}

func TestMatchByTokenThenMSGIDThenSender(t *testing.T) {
	open := openPings()
	if p := Match(Reply{Subject: "Re: ping E5F6A7B8", FromAddr: "1:1/0"}, open); p == nil || p.Address != "1:1/19" {
		t.Errorf("token match failed: %+v", p)
	}
	if p := Match(Reply{Body: "  MSGID: 2:5001/100 68b8a1c2\n", FromAddr: "2:2/0"}, open); p == nil || p.Address != "2:280/5555" {
		t.Errorf("quoted MSGID match failed: %+v", p)
	}
	if p := Match(Reply{FromAddr: "2:280/5555", Subject: "hello"}, open); p == nil || p.Address != "2:280/5555" {
		t.Errorf("sole open ping to the sender must match: %+v", p)
	}
	// Two open pings to the sender: the one still waiting wins, else the newest.
	if p := Match(Reply{FromAddr: "2:221/1", Subject: "hello"}, open); p == nil || p.Mode != ModeDirect {
		t.Errorf("with equal status the newest ping wins, got %+v", p)
	}
	open[3].Status = StatusPong
	if p := Match(Reply{FromAddr: "2:221/1", Subject: "hello"}, open); p == nil || p.Mode != "" {
		t.Errorf("the unanswered ping wins over an answered one, got %+v", p)
	}
	if p := Match(Reply{FromAddr: "2:9999/1", Subject: "hello"}, open); p != nil {
		t.Errorf("stranger must not match, got %+v", p)
	}
}

func TestClassify(t *testing.T) {
	p := &Ping{Address: "2:280/5555"}
	path := []Hop{{Address: "2:5001/100"}, {Address: "2:5020/715"}, {Address: "2:280/5555"}}
	cases := []struct {
		name string
		r    Reply
		p    *Ping
		want string
	}{
		{"ndr wins", Reply{FromName: "NDR Robot", FromAddr: "2:5001/100", Subject: "Undeliverable: PING"}, p, KindNDR},
		{"unmatched", Reply{FromName: "Someone", FromAddr: "2:1/1"}, nil, KindUnmatched},
		{"from target is pong", Reply{FromName: "Ping Robot", FromAddr: "2:280/5555", Subject: "whatever"}, p, KindPong},
		{"trace wording", Reply{FromName: "Trace Robot", FromAddr: "2:5020/715", Subject: "Trace: your message to PING"}, p, KindTrace},
		{"pong wording from an AKA", Reply{FromName: "Robot", FromAddr: "2:280/1", Subject: "Your message arrived"}, p, KindPong},
		{"intermediate without wording is trace", Reply{FromName: "Robot", FromAddr: "2:5020/715", Subject: "PING"}, p, KindTrace},
		{"unknown sender without wording is pong", Reply{FromName: "Robot", FromAddr: "2:280/2", Subject: "PING"}, p, KindPong},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.r, tc.p, path); got != tc.want {
				t.Errorf("got %s want %s", got, tc.want)
			}
		})
	}
}
