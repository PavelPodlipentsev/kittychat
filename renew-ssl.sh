#!/bin/bash
certbot certonly --manual --preferred-challenges dns \
  --manual-auth-hook "/home/lenovo/regru-auth.sh" \
  --manual-cleanup-hook "/home/lenovo/regru-cleanup.sh" \
  -d wonderfulzoka.online --non-interactive --agree-tos \
  -m dertdert2003@gmail.com --force-renewal

sudo systemctl reload nginx
echo "SSL renewed at $(date)" >> /home/lenovo/ssl-renew.log
