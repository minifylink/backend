# backend

go/psql endpoints
```
// https::/.../add
post
request = {
  url: "URL" // string
}
answer = {
  state: STATE // state_enum
  state_msg: "MSG" // string
  short_url_if_possible: "URL" // string
}

// https::/.../stat
post
request = {
  url: "URL" // string
}
answer = {
  state: STATE // state_enum
  state_msg: "MSG" // string
  stat_url_if_possible: 000 // unsigned int
}

enum state_enum = {
  SUCCESS
  ERROR
}
```
