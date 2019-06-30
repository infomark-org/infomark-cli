// InfoMark - a platform for managing courses with
//            distributing exercise sheets and testing exercise submissions
// Copyright (C) 2019  Patrick Wieschollek
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.

// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package bridge

import (
  "bufio"
  "bytes"
  "encoding/json"
  "fmt"
  "io"
  "io/ioutil"
  "log"
  "mime/multipart"
  "net/http"
  "net/textproto"
  "os"
  "path/filepath"
  "strings"
  "syscall"

  "github.com/cgtuebingen/infomark-backend/api/app"
  "github.com/fatih/color"
  "golang.org/x/crypto/ssh/terminal"
)

// H is a neat alias
type H map[string]interface{}

type Connection struct {
  Email       string
  URL         string
  Credentials app.AuthResponse
}

func (c *Connection) RequireEmail() {
  email := os.Getenv("INFOMARK_EMAIL")
  if c.Email == "" {
    if email == "" {
      fmt.Println(`No email is given, you might want to add your email to the
environment variable "INFOMARK_EMAIL"`)
      reader := bufio.NewReader(os.Stdin)
      fmt.Print("Enter Email: ")
      tmp, _ := reader.ReadString('\n')
      email = strings.TrimSpace(tmp)
    }
  }
  c.Email = email
}

func (c *Connection) RequireURL() {
  url := os.Getenv("INFOMARK_URL")
  if c.URL == "" {
    if url == "" {
      fmt.Println(`No url is given, you might want to add the endpoint url to the
environment variable "INFOMARK_URL"`)
      reader := bufio.NewReader(os.Stdin)
      fmt.Print("Enter URL: ")
      tmp, _ := reader.ReadString('\n')
      url = strings.TrimSpace(tmp)
    }
  }
  c.URL = url
}

func (c *Connection) RequireCredentials() {
  c.RequireEmail()
  c.RequireURL()

  if c.Credentials.Access.Token == "" {
    pass := os.Getenv("INFOMARK_PASSWORD")
    fmt.Println(`No password is given, you might want to add the password to the
environment variable "INFOMARK_PASSWORD"`)
    if pass == "" {
      fmt.Print("Enter Password: ")
      bytePassword, err := terminal.ReadPassword(int(syscall.Stdin))
      if err != nil {
        panic(err)
      }
      fmt.Printf("\n")
      pass = strings.TrimSpace(string(bytePassword))
    }

    b := &Bridge{Connection: &Connection{URL: c.URL}}
    w := b.Post("/api/v1/auth/token",
      H{
        "email":          c.Email,
        "plain_password": pass,
      },
    )

    if w.OK() {
      credentials := app.AuthResponse{}
      w.DecodeJSON(&credentials)
      c.Credentials = credentials
    }

  }

}

func NewConnection() Connection {
  conn := Connection{}
  conn.RequireURL()
  return conn
}

type Bridge struct {
  Client     http.Client
  Connection *Connection
  Verbose    bool
}

type ParsedResponse struct {
  Response *http.Response
}

func (pr *ParsedResponse) Plain() string {
  body, _ := ioutil.ReadAll(pr.Response.Body)
  return string(body)
}

func (pr *ParsedResponse) DecodeJSON(target interface{}) error {
  return json.NewDecoder(pr.Response.Body).Decode(target)
}

func (pr *ParsedResponse) PrintCode() {
  switch pr.Response.StatusCode {
  case 200, 201, 202, 203, 204:
    color.Green(pr.Response.Status)
  case 400, 401, 403, 404:
    color.Yellow(pr.Response.Status)
  case 500:
    color.Red(pr.Response.Status)
  default:
    fmt.Println(pr.Response.Status)
  }
}

func (pr *ParsedResponse) OK() bool {
  switch pr.Response.StatusCode {
  case 200, 201, 202, 203, 204:
    return true
  default:
    return false
  }
}

func (pr *ParsedResponse) Close() {
  pr.Response.Body.Close()
}

// NewBridge creates a new Bridge
func NewBridge() *Bridge {
  return &Bridge{Client: http.Client{}}
}

type RequestModifier interface {
  Modify(r *http.Request)
}

func (c *Connection) Modify(r *http.Request) {
  r.Header.Add("Authorization", "Bearer "+c.Credentials.Access.Token)
}

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

func escapeQuotes(s string) string {
  return quoteEscaper.Replace(s)
}

// ugly!! but see https://github.com/golang/go/issues/16425
func createFormFile(w *multipart.Writer, fieldname, filename string, contentType string) (io.Writer, error) {
  h := make(textproto.MIMEHeader)
  h.Set("Content-Disposition",
    fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
      escapeQuotes(fieldname), escapeQuotes(filename)))
  h.Set("Content-Type", contentType)
  return w.CreatePart(h)
}

// CreateFileRequestBody create a multi-part form data. We assume all endpoints
// handling files are receicing a single file with form name "file_data"
func CreateFileRequestBody(path, contentType string, params map[string]string) (*bytes.Buffer, string, error) {
  // open file on disk
  file, err := os.Open(path)
  if err != nil {
    return nil, "", err
  }
  defer file.Close()

  // create body
  body := &bytes.Buffer{}
  writer := multipart.NewWriter(body)
  part, err := createFormFile(writer, "file_data", filepath.Base(path), contentType)
  if err != nil {
    return nil, "", err
  }
  _, err = io.Copy(part, file)
  if err != nil {
    return nil, "", err
  }

  for key, val := range params {
    err = writer.WriteField(key, val)
    if err != nil {
      return nil, "", err
    }
  }

  err = writer.Close()
  if err != nil {
    return nil, "", err
  }

  return body, writer.FormDataContentType(), nil

}

// BuildDataRequest creates a request
func (b Bridge) BuildDataRequest(method, url string, data map[string]interface{}) *http.Request {

  var payloadJson *bytes.Buffer

  if data != nil {
    dat, err := json.Marshal(data)
    if err != nil {
      panic(err)
    }
    payloadJson = bytes.NewBuffer(dat)
  } else {
    payloadJson = nil
  }

  r, err := http.NewRequest(method, fmt.Sprintf("%s%s", b.Connection.URL, url), payloadJson)
  if err != nil {
    panic(err)
  }

  r.Header.Set("Content-Type", "application/json")
  // r.Header.Add("X-Forwarded-For", "1.2.3.4")
  r.Header.Set("User-Agent", "Infomark-Cli")

  return r
}

// PlayRequest will send the request to the router and fetch the response
func (b *Bridge) PlayRequest(r *http.Request) *ParsedResponse {
  if b.Verbose {
    color.Cyan(b.FormatRequest(r))

  }
  resp, err := b.Client.Do(r)
  if err != nil {
    log.Fatal(color.RedString("End point cannot be reached, are you online?"))
  }
  rp := &ParsedResponse{Response: resp}
  if b.Verbose {
    rp.PrintCode()
  }
  return rp
}

// ToH is a convenience wrapper create a json for any object
func ToH(z interface{}) map[string]interface{} {
  data, _ := json.Marshal(z)
  var msgMapTemplate interface{}
  _ = json.Unmarshal(data, &msgMapTemplate)
  return msgMapTemplate.(map[string]interface{})
}

// ToH is a convenience wrapper create a json for any object
func (b *Bridge) ToH(z interface{}) map[string]interface{} {
  data, _ := json.Marshal(z)
  var msgMapTemplate interface{}
  _ = json.Unmarshal(data, &msgMapTemplate)
  return msgMapTemplate.(map[string]interface{})
}

// Get creates, sends a GET request and fetches the response
func (b *Bridge) Get(url string, modifiers ...RequestModifier) *ParsedResponse {
  h := make(map[string]interface{})
  r := b.BuildDataRequest("GET", url, h)

  for _, modifier := range modifiers {
    modifier.Modify(r)
  }

  return b.PlayRequest(r)
}

// Post creates, sends a POST request and fetches the response
func (b *Bridge) Post(url string, data map[string]interface{}, modifiers ...RequestModifier) *ParsedResponse {
  r := b.BuildDataRequest("POST", url, data)

  for _, modifier := range modifiers {
    modifier.Modify(r)
  }

  return b.PlayRequest(r)
}

// Put creates, sends a PUT request and fetches the response
func (b *Bridge) Put(url string, data map[string]interface{}, modifiers ...RequestModifier) *ParsedResponse {
  r := b.BuildDataRequest("PUT", url, data)

  for _, modifier := range modifiers {
    modifier.Modify(r)
  }

  return b.PlayRequest(r)
}

// Patch creates, sends a PATCH request and fetches the response
func (b *Bridge) Patch(url string, data map[string]interface{}, modifiers ...RequestModifier) *ParsedResponse {
  r := b.BuildDataRequest("PATCH", url, data)

  for _, modifier := range modifiers {
    modifier.Modify(r)
  }

  return b.PlayRequest(r)
}

// Delete creates, sends a DELETE request and fetches the response
func (b *Bridge) Delete(url string, modifiers ...RequestModifier) *ParsedResponse {
  h := make(map[string]interface{})
  r := b.BuildDataRequest("DELETE", url, h)

  for _, modifier := range modifiers {
    modifier.Modify(r)
  }

  return b.PlayRequest(r)
}

// FormatRequest pretty-formats a request and returns it as a string
func (b *Bridge) FormatRequest(r *http.Request) string {
  // Create return string
  var request []string
  // Add the request string
  url := fmt.Sprintf("%v %v %v", r.Method, r.URL, r.Proto)
  request = append(request, url)
  // Add the host
  request = append(request, fmt.Sprintf("Host: %v", r.Host))
  // Loop through headers
  for name, headers := range r.Header {
    name = strings.ToLower(name)
    for _, h := range headers {
      request = append(request, fmt.Sprintf("%v: %v", name, h))
    }
  }

  // If this is a POST, add post data
  if r.Method == "POST" {
    r.ParseForm()
    request = append(request, "\n")
    request = append(request, r.Form.Encode())
  }
  // Return the request as a string
  return strings.Join(request, "\n")
}

func (b *Bridge) Upload(url string, filename string, contentType string, modifiers ...RequestModifier) (*ParsedResponse, error) {
  return b.UploadWithParameters(url, filename, contentType, map[string]string{}, modifiers...)
}

func (b *Bridge) UploadWithParameters(url string, filename string, contentType string, params map[string]string, modifiers ...RequestModifier) (*ParsedResponse, error) {

  body, ct, err := CreateFileRequestBody(filename, contentType, params)
  if err != nil {
    return nil, err
  }

  r, err := http.NewRequest("POST", fmt.Sprintf("%s%s", b.Connection.URL, url), body)
  if err != nil {
    return nil, err
  }
  r.Header.Set("Content-Type", ct)
  // r.Header.Add("X-Forwarded-For", "1.2.3.4")
  r.Header.Set("User-Agent", "Infomark-Cli")

  for _, modifier := range modifiers {
    modifier.Modify(r)
  }

  return b.PlayRequest(r), nil
}
