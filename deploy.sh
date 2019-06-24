cd ~/webapp/go
make
sudo systemctl restart nginx
sudo systemctl restart isuda.go
cd ~/isucon6q
jq < ./isucon6q-bench
cd ~
