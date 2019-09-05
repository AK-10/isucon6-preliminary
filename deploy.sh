cd ~/webapp/go
make
sudo systemctl restart nginx
sudo systemctl restart isuda.go
sudo systemctl restart isutar.go
sudo systemctl restart redis
cd ~/isucon6q
./isucon6q-bench
cd ~
