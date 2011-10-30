from BeautifulSoup import BeautifulStoneSoup, SoupStrainer

class InvalidFB2(Exception):
    pass


class FB2MetadataCollector(object):
    BYTES_LIMIT = 2048

    def __init__(self, data):
        self.data = data

    @classmethod
    def create_instance_by_filename(cls, filename):
        with open(filename) as file:
            return cls.create_instance_by_file(file)

    @classmethod
    def create_instance_by_file(cls, file):
        data = file.read(cls.BYTES_LIMIT)
        return cls(data)

    def get_author_first_name(self):
            return self.get_author_data().find('first-name').text

    def get_author_middle_name(self):
            return self.get_author_data().find('middle-name').text

    def get_author_last_name(self):
            return self.get_author_data().find('last-name').text

    def get_title(self):
        return self.get_title_info().find('book-title').text

    def get_genre(self):
        return self.get_title_info().find('genre').text

    def get_date(self):
        return self.get_title_info().find('date').text

    def get_id(self):
        return self.get_title_info().find('id').text

    def get_lang(self):
        return self.get_title_info().find('lang').text

    def get_author_data(self):
        return self.get_title_info().find('author')

    def get_title_info(self):
        if not hasattr(self, 'title_info'):
            try:
                self.title_info = self.get_soup(only='title-info').find('title-info').extract()
            except AttributeError:
                raise InvalidFB2
        return self.title_info

    def get_soup(self, only=None):
        return BeautifulStoneSoup(
            self.data,
            parseOnlyThese=SoupStrainer(only),
            markupMassage=False,
        )