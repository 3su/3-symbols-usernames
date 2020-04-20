package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/go-redis/redis/v7"
	"github.com/go-resty/resty/v2"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Service struct {
	Redis *redis.Client
	Resty *resty.Request
}

var ListExceptionWords = []string{"any", "api", "lab", "cla", "edu", "mac", "man", "new", "raw", "ssh", "ten", "tos"}

var (
	UseRedis           *bool
	RedisAddr          *string
	RedisPassword      *string
	RedisDB            *int
	RecheckExistLogins *bool
	RecheckFreeLogins  *bool
	Tokens             []string
)

func main() {
	rand.Seed(time.Now().UnixNano())

	UseRedis = flag.Bool("use_redis", false, "check true if you use redis for interim results")
	RedisAddr = flag.String("redis_addr", "localhost:6379", "redis host:port, default localhost:6379")
	RedisPassword = flag.String("redis_password", "", "type redis password, default no password")
	RedisDB = flag.Int("redis_db", 0, "type redis db, default 0")

	RecheckExistLogins = flag.Bool("recheck_exists", false, "check true if you need to recheck exist logins")
	RecheckFreeLogins = flag.Bool("recheck_free", false, "check true if you need to recheck free logins")

	tokens := flag.String("tokens", "", "tokens separated by \",\"")

	flag.Parse()

	Tokens = strings.Split(*tokens, ",")

	s := &Service{}

	if *UseRedis {
		s.Redis = ExampleNewClient()
	}

	rClient := resty.New()
	request := rClient.R()

	s.Resty = request

	template := "abcdefghijklmnopqrstuvwxyz0123456789"

	var loginList string
	loginList = "### All free 3 symbols logins for github.com\n\n"
	loginList += "#### Use flags:\n"
	loginList += "- **-use_redis** - `bool`, default false;\n"
	loginList += "- **-redis_addr** - `string`, host:port, default localhost:6379;\n"
	loginList += "- **-redis_password** - `string`, default \"\";\n"
	loginList += "- **-redis_db** - `int`, default 0;\n"
	loginList += "- **-recheck_exists** - `bool`, default false;\n"
	loginList += "- **-recheck_free** - `bool`, default false;\n"
	loginList += "- **-tokens** - `string` separated by \",\";\n"
	loginList += "\n\n"

	for _, ch := range template {

		loginList += "\n\n"
		loginList +=  strings.ToUpper(fmt.Sprintf("###### %c: \n",ch))

		for _, ch2 := range template {
			for _, ch3 := range template {
				name := fmt.Sprintf("%c%c%c", ch, ch2, ch3)

				err := s.checkName(name)
				if err != nil {
					log.Println(name, err)
					continue
				}

				loginList += name + " "

			}
		}
	}

	err := ioutil.WriteFile("README.md", []byte(loginList), 0644)
	if err != nil {
		log.Fatal()
	}

	fmt.Println("Все запросы выполнены")
}

func (s *Service) checkName(name string) error {
	for i := 0; i < len(ListExceptionWords); i++ {
		if name == ListExceptionWords[i] {
			return errors.New("exception word")
		}
	}

	if len(Tokens) > 0 {
		s.Resty.SetAuthToken(Tokens[rand.Intn(len(Tokens))])
	}

	freeKey := fmt.Sprintf("github:login:%d:FREE:%s", len(name), name)
	existKey := fmt.Sprintf("github:login:%d:EXIST:%s", len(name), name)

	if *UseRedis && !*RecheckFreeLogins {
		redisFree, err := s.Redis.Exists(freeKey).Result()
		if err != nil {
			return fmt.Errorf("redis free-key exists error: %w", err)
		}
		if redisFree > 0 {
			return nil
		}
	}

	if *UseRedis && !*RecheckExistLogins {
		redisExists, err := s.Redis.Exists(existKey).Result()
		if err != nil {
			return fmt.Errorf("redis exist-key exists error: %w", err)
		}
		if redisExists > 0 {
			return errors.New("login exist")
		}
	}

	response, err := s.Resty.Get("https://api.github.com/users/" + name)
	if err != nil {
		return fmt.Errorf("resty get error: %w", err)
	}

	if err := s.checkRateLimit(response); err != nil {
		return err
	}

	if response.StatusCode() == http.StatusOK {
		if *UseRedis {
			if err := s.Redis.Incr(existKey).Err(); err != nil {
				return fmt.Errorf("redis exist-key incr error: %w", err)
			}
		}
		return errors.New("login exist")
	}

	if *UseRedis {
		if err := s.Redis.Incr(freeKey).Err(); err != nil {
			return fmt.Errorf("redis free-key incr error: %w", err)
		}
	}

	log.Println(">>>>>>>>>>>>> free login", name)

	return nil
}

func (s *Service) checkRateLimit(response *resty.Response) error {
	rateLimit := response.Header().Get("X-RateLimit-Remaining")
	rLimit, err := strconv.Atoi(rateLimit)
	if err != nil {
		return fmt.Errorf("strconv atoi for \"X-RateLimit-Remaining\" error: %w", err)
	}
	rateReset := response.Header().Get("X-RateLimit-Reset")
	rReset, err := strconv.Atoi(rateReset)
	if err != nil {
		return fmt.Errorf("strconv atoi for \"X-RateLimit-Reset\" error: %w", err)
	}
	sleepTime := int32(rReset) - int32(time.Now().Unix())
	var logs string

	logs += fmt.Sprintf("limit left = %d, sleepTime min = %d", rLimit, sleepTime/60)
	if rLimit <= 0 && sleepTime > 0 {
		log.Println(logs)
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}

	log.Println(logs)

	return nil
}

func ExampleNewClient() *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr:     *RedisAddr,
		Password: *RedisPassword,
		DB:       *RedisDB,
	})

	pong, err := client.Ping().Result()
	if err != nil {
		log.Fatal(err)
	}

	if len(pong) < 0 {
		log.Fatal(err)
	}

	return client
}
