# Credentials Directory

This directory holds sensitive Firebase credentials for the backend services.

## Required Files

### firebase-key.json
Your Firebase service account key. This file is required for:
- Firebase Authentication token verification
- Firestore database access
- Firebase Admin SDK operations

## Setup Instructions

### Local Development
Copy your Firebase service account key here:
```bash
cp /path/to/trustlink-1bae8-firebase-adminsdk-*.json ./credentials/firebase-key.json
```

### Digital Ocean Deployment
Transfer the credentials securely using SCP:
```bash
# From your local machine
scp trustlink-1bae8-firebase-adminsdk-*.json \
  user@YOUR_DROPLET_IP:/opt/trustlink/backend/credentials/firebase-key.json
```

## Security Notes

⚠️ **NEVER commit credentials to Git!**

- This directory is included in `.gitignore`
- Credentials should be transferred securely
- Use environment variables or secrets management in production
- Rotate keys if accidentally exposed

## File Permissions

Ensure proper permissions:
```bash
chmod 600 credentials/firebase-key.json
```

## Verification

Check if the file exists and is readable:
```bash
ls -lh credentials/firebase-key.json
```

Expected output:
```
-rw------- 1 user user 2.4K Feb 04 21:00 credentials/firebase-key.json
```
