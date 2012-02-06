from BeautifulSoup import BeautifulStoneSoup, SoupStrainer

class InvalidFB2(Exception):
    pass


class FB2MetadataCollector(object):
    BYTES_LIMIT = 2048

    def __init__(self, data):
        self.data = data
        self._title_info = self.__get_title_info()
        self._author_data = self.__get_author_data()
        if not self._title_info or not self._author_data:
            raise InvalidFB2

    @classmethod
    def create_instance_by_filename(cls, filename):
        with open(filename) as file:
            return cls.create_instance_by_file(file)

    @classmethod
    def create_instance_by_file(cls, file):
        data = file.read(cls.BYTES_LIMIT)
        return cls(data)

    def get_author_first_name(self):
        return self._get_author_data_block_text_or_None('first-name')

    def get_author_middle_name(self):
        return self._get_author_data_block_text_or_None('middle-name')

    def get_author_last_name(self):
        return self._get_author_data_block_text_or_None('last-name')

    def get_title(self):
        return self._get_title_info_block_text_or_None('book-title')

    def get_genre(self):
        return self._get_title_info_block_text_or_None('genre')

    def get_date(self):
        return self._get_title_info_block_text_or_None('date')

    def get_id(self):
        return self._get_title_info_block_text_or_None('id')

    def get_language(self):
        return self._get_title_info_block_text_or_None('lang')

    def _get_title_info_block_text_or_None(self, block_name):
        block = self._title_info.find(block_name)
        if block:
            return block.text

    def _get_author_data_block_text_or_None(self, block_name):
        block = self._author_data.find(block_name)
        if block:
            return block.text

    def __get_author_data(self):
        return self.__get_title_info().find('author')

    def __get_title_info(self):
        if not hasattr(self, 'title_info'):
            try:
                self.title_info = self.__get_soup(only='title-info').find('title-info').extract()
            except AttributeError:
                raise InvalidFB2
        return self.title_info

    def __get_soup(self, only=None):
        return BeautifulStoneSoup(
            self.data,
            parseOnlyThese=SoupStrainer(only),
            markupMassage=False,
        )