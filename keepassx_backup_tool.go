package main

import (
  "encoding/json"
  "fmt"
  "io/ioutil"
  "log"
  "net/http"
  "net/url"
  "os"
  "os/user"
  "path/filepath"

  "golang.org/x/net/context"
  "golang.org/x/oauth2"
  "golang.org/x/oauth2/google"
  "google.golang.org/api/drive/v3"

  "crypto/md5"
  "io"
  "encoding/hex"
)

// getClient uses a Context and Config to retrieve a Token
// then generate a Client. It returns the generated Client.
func getClient(ctx context.Context, config *oauth2.Config) *http.Client {
  cacheFile, err := tokenCacheFile()
  if err != nil {
    log.Fatalf("Unable to get path to cached credential file. %v", err)
  }
  tok, err := tokenFromFile(cacheFile)
  if err != nil {
    tok = getTokenFromWeb(config)
    saveToken(cacheFile, tok)
  }
  return config.Client(ctx, tok)
}

// getTokenFromWeb uses Config to request a Token.
// It returns the retrieved Token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
  authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
  fmt.Printf("Go to the following link in your browser then type the "+
    "authorization code: \n%v\n", authURL)

  var code string
  if _, err := fmt.Scan(&code); err != nil {
    log.Fatalf("Unable to read authorization code %v", err)
  }

  tok, err := config.Exchange(oauth2.NoContext, code)
  if err != nil {
    log.Fatalf("Unable to retrieve token from web %v", err)
  }
  return tok
}

// tokenCacheFile generates credential file path/filename.
// It returns the generated credential path/filename.
func tokenCacheFile() (string, error) {
  usr, err := user.Current()
  if err != nil {
    return "", err
  }
  tokenCacheDir := filepath.Join(usr.HomeDir, ".credentials", "keepassx_backup")
  os.MkdirAll(tokenCacheDir, 0700)
  return filepath.Join(tokenCacheDir,
    url.QueryEscape("drive-go-keepassx-backup.json")), err
}

// tokenFromFile retrieves a Token from a given file path.
// It returns the retrieved Token and any read error encountered.
func tokenFromFile(file string) (*oauth2.Token, error) {
  f, err := os.Open(file)
  if err != nil {
    return nil, err
  }
  t := &oauth2.Token{}
  err = json.NewDecoder(f).Decode(t)
  defer f.Close()
  return t, err
}

// saveToken uses a file path to create a file and store the
// token in it.
func saveToken(file string, token *oauth2.Token) {
  fmt.Printf("Saving credential file to: %s\n", file)
  f, err := os.Create(file)
  if err != nil {
    log.Fatalf("Unable to cache oauth token: %v", err)
  }
  defer f.Close()
  json.NewEncoder(f).Encode(token)
}

func main() {
  ctx := context.Background()

  log.Println("Beginning of syncing")

  if len(os.Args) != 3 {
    log.Fatalf("Please provide .kdbx file path and client secret file path as arguments!")
  }

  localRingFilePath := os.Args[1]
  clientSecretFilePath := os.Args[2]
  ringFileName := filepath.Base(localRingFilePath)

  b, err := ioutil.ReadFile(clientSecretFilePath)
  if err != nil {
    log.Fatalf("Unable to read client secret file: %v", err)
  }

  // If modifying these scopes, delete your previously saved credentials
  // at ~/.credentials/keepassx_backup/drive-go-keepassx-backup.json
  config, err := google.ConfigFromJSON(b, drive.DriveFileScope)
  if err != nil {
    log.Fatalf("Unable to parse client secret file to config: %v", err)
  }
  client := getClient(ctx, config)

  srv, err := drive.New(client)
  if err != nil {
    log.Fatalf("Unable to retrieve drive Client %v", err)
  }

  queryString := "mimeType = 'application/vnd.google-apps.folder' and name = 'automatic_backups' and 'root' in parents"
  r, err := srv.Files.List().Fields("files(id)").Q(queryString).Do()

  if err != nil {
    log.Fatalf("Unable to retrieve files: %v", err)
  }

  log.Println("Checking for automatic_backups folder existence:")

  var backupsFolderId string

  if len(r.Files) > 0 {
    backupsFolderId = r.Files[0].Id
  } else {

    log.Println("Creating automatic_backups folder")
    myFile := drive.File{ Name: "automatic_backups", MimeType: "application/vnd.google-apps.folder" }
    f, err := srv.Files.Create(&myFile).Do()

    if err != nil {
      log.Fatalf("Unable to create automatic_backups folder: %v", err)
    }

    backupsFolderId = f.Id
  }

  ringFile, err := os.Open(localRingFilePath)
  defer ringFile.Close()

  if err != nil {
    log.Fatalf("Unable to open .kdbx file: %v", err)
  }

  // calculate md5 hash of .kdbx file on HDD
  hash := md5.New()
  _, err = io.Copy(hash, ringFile)

  if err != nil {
    log.Fatalf("Unable to calculate md5 hash of .kdbx file: %v", err)
  }

  ringFileHash := hex.EncodeToString(hash.Sum(nil))
  ringFile.Seek(0,0) // reset file reading offset after io.Copy operation

  // if .kdbx is empty file, by comparing to md5("")
  if ringFileHash == "d41d8cd98f00b204e9800998ecf8427e" {
    log.Fatalf("File .kdbx is empty")
  }

  queryString = fmt.Sprintf("name = '%s' and '%s' in parents", ringFileName, backupsFolderId)
  r, err = srv.Files.List().Fields("files(id, md5Checksum)").Q(queryString).Do()

  if err != nil {
    log.Fatalf("Unable to retrieve files: %v", err)
  }

  log.Println("Checking for .kdbx file existence on Drive:")

  if len(r.Files) > 0 {
    // if .kdbx file has changed since last syncing
    if (r.Files[0].Md5Checksum != ringFileHash) {
      log.Println("Updating .kdbx file")
      ringFileId := r.Files[0].Id

      myFile := drive.File{ Name: ringFileName }
      f, err := srv.Files.Update(ringFileId, &myFile).Media(ringFile).Do()

      if err != nil {
        log.Fatalf("Unable to create automatic_backups fodler: %v", err)
      }

      log.Println("Successfully updated .kdbx file, id: ", f.Id)
    } else {
      log.Println("The passwords file has not been changed since last sync")
    }
  } else {
    log.Println("Creating .kdbx file")
    myFile := drive.File{ Name: ringFileName, Parents: []string{ backupsFolderId } }

    // create new .kdbx file
    f, err := srv.Files.Create(&myFile).Media(ringFile).Do()

    if err != nil {
      log.Fatalf("Unable to create .kdbx: %v", err)
    }

    log.Println("Successfully created .kdbx file, id: ", f.Id)
  }

  log.Println("End of syncing")
  fmt.Print("\n\n")
}
