"""Integration tests for the Models API."""


class TestModels:
    def test_list_models(self, client):
        result = client.models.list()
        models = list(result)
        assert len(models) > 0
        for model in models:
            assert model.id
            assert model.object == "model"

    def test_retrieve_model(self, client):
        models = list(client.models.list())
        assert len(models) > 0

        first = models[0]
        retrieved = client.models.retrieve(first.id)
        assert retrieved.id == first.id
        assert retrieved.object == "model"
