# KeePassX Backup Tool

## Setup instructions

1. Compile backup_tools, and add it's location to $PATH
2. Follow instructions from: https://developers.google.com/drive/v3/web/quickstart/go and save client_secret.json file
3. Run application with aruments, e.g. keepassx_backup_tool /home/sampleuser/ring.kdbx /home/sampleuser/Downloads/client_secret.json where 1st argument is path to KeePassX, and 2nd is path of file downloaded in step 2
4. Open displayed authorization link in browser and allow access
