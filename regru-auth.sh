#!/bin/bash
REGRU_USER="dertdert2003@gmail.com"
REGRU_PASS="Zoka2.0.0"
DOMAIN="wonderfulzoka.online"

curl -s "https://api.reg.ru/api/regru2/zone/add_txt" \
  -d "username=$REGRU_USER" \
  -d "password=$REGRU_PASS" \
  -d "domains[0][dname]=$DOMAIN" \
  -d "subdomain=_acme-challenge" \
  -d "text=$CERTBOT_VALIDATION" \
  -d "output_content_type=plain"

sleep 30
