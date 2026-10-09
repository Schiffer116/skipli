cicd:
	act -W .github/workflows/ci.yaml $(ARGS)

deploy-%:
	./infra/deploy.sh $* $(ARGS)
